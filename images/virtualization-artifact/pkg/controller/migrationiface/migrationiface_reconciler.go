/*
Copyright 2026 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package migrationiface

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
)

const (
	sdnGroup         = "network.deckhouse.io"
	sdnVersion       = "v1alpha1"
	sdnNodeNameLabel = sdnGroup + "/node-name"
	sdnInterfaceType = sdnGroup + "/interface-type"
	sdnInterfaceVLAN = "VLAN"
)

var (
	snnniaGVK = schema.GroupVersionKind{Group: sdnGroup, Version: sdnVersion, Kind: "SystemNetworkNodeNetworkInterfaceAttachment"}
	nniGVK    = schema.GroupVersionKind{Group: sdnGroup, Version: sdnVersion, Kind: "NodeNetworkInterface"}
)

func NewReconciler(c client.Client, systemNetworkName string, log *log.Logger) *Reconciler {
	return &Reconciler{
		client:            c,
		systemNetworkName: systemNetworkName,
		log:               log,
	}
}

type Reconciler struct {
	client            client.Client
	systemNetworkName string
	log               *log.Logger
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	nodePredicate := predicate.TypedFuncs[*corev1.Node]{
		CreateFunc: func(event.TypedCreateEvent[*corev1.Node]) bool { return true },
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Node]) bool {
			return e.ObjectOld.Annotations[annotations.AnnMigrationIface] !=
				e.ObjectNew.Annotations[annotations.AnnMigrationIface]
		},
		DeleteFunc:  func(event.TypedDeleteEvent[*corev1.Node]) bool { return false },
		GenericFunc: func(event.TypedGenericEvent[*corev1.Node]) bool { return false },
	}
	if err := ctr.Watch(source.Kind(mgr.GetCache(),
		&corev1.Node{},
		&handler.TypedEnqueueRequestForObject[*corev1.Node]{},
		nodePredicate,
	)); err != nil {
		return fmt.Errorf("watch Node: %w", err)
	}

	r.watchSdnKind(mgr, ctr, snnniaGVK, func(obj *unstructured.Unstructured) string {
		n, _, _ := unstructured.NestedString(obj.Object, "status", "nodeName")
		return n
	})

	r.watchSdnKind(mgr, ctr, nniGVK, func(obj *unstructured.Unstructured) string {
		if obj.GetLabels()[sdnInterfaceType] != sdnInterfaceVLAN {
			return ""
		}
		if n := obj.GetLabels()[sdnNodeNameLabel]; n != "" {
			return n
		}
		n, _, _ := unstructured.NestedString(obj.Object, "spec", "nodeName")
		return n
	})

	return nil
}

func (r *Reconciler) watchSdnKind(
	mgr manager.Manager,
	ctr controller.Controller,
	gvk schema.GroupVersionKind,
	toNodeName func(*unstructured.Unstructured) string,
) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	err := ctr.Watch(source.Kind(mgr.GetCache(), obj,
		handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, o *unstructured.Unstructured) []reconcile.Request {
			if n := toNodeName(o); n != "" {
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: n}}}
			}
			return nil
		}),
	))
	if err != nil {
		r.log.Warn("sdn watch failed; migration interface annotation will not track sdn changes",
			"kind", gvk.Kind, "err", err.Error())
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var node corev1.Node
	if err := r.client.Get(ctx, req.NamespacedName, &node); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	desired, err := r.resolveInterfaceForNode(ctx, node.Name)
	if err != nil {
		return reconcile.Result{}, err
	}

	if node.Annotations[annotations.AnnMigrationIface] == desired {
		return reconcile.Result{}, nil
	}

	var value any // nil → annotation removed
	if desired != "" {
		value = desired
	}
	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{annotations.AnnMigrationIface: value},
		},
	})
	if err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Patch(ctx, &node, client.RawPatch(types.StrategicMergePatchType, patch)); err != nil {
		return reconcile.Result{}, fmt.Errorf("patch node %q annotation: %w", node.Name, err)
	}

	r.log.Info("updated migration interface annotation",
		"node", node.Name,
		"systemNetwork", r.systemNetworkName,
		"interface", desired,
	)
	return reconcile.Result{}, nil
}

func (r *Reconciler) resolveInterfaceForNode(ctx context.Context, nodeName string) (string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Group: sdnGroup, Version: sdnVersion, Kind: snnniaGVK.Kind + "List"})
	err := r.client.List(ctx, list, client.MatchingFields{
		indexer.IndexFieldSNNNIAByNodeName:          nodeName,
		indexer.IndexFieldSNNNIABySystemNetworkName: r.systemNetworkName,
	})
	if err != nil {
		if meta.IsNoMatchError(err) {
			return "", nil
		}
		return "", fmt.Errorf("list %s: %w", snnniaGVK.Kind, err)
	}

	for i := range list.Items {
		nniName, _, _ := unstructured.NestedString(list.Items[i].Object, "status", "nodeNetworkInterfaceName")
		if nniName == "" {
			continue
		}
		return r.ifNameFromNNI(ctx, nniName)
	}
	return "", nil
}

func (r *Reconciler) ifNameFromNNI(ctx context.Context, nniName string) (string, error) {
	nni := &unstructured.Unstructured{}
	nni.SetGroupVersionKind(nniGVK)
	if err := r.client.Get(ctx, client.ObjectKey{Name: nniName}, nni); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return "", nil
		}
		return "", fmt.Errorf("get %s %q: %w", nniGVK.Kind, nniName, err)
	}
	ifName, _, _ := unstructured.NestedString(nni.Object, "status", "ifName")
	return ifName, nil
}
