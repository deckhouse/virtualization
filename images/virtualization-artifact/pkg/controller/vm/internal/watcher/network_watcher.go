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

package watcher

import (
	"context"
	"fmt"

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
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// NewNetworkWatcher enqueues VMs that reference a Network or ClusterNetwork
// when that network's Ready condition changes.
func NewNetworkWatcher(c client.Client) *NetworkWatcher {
	return &NetworkWatcher{client: c}
}

type NetworkWatcher struct {
	client client.Client
}

func (w *NetworkWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := w.watchOne(mgr, ctr, commonnetwork.ClusterNetworkGVK, indexer.IndexFieldVMByClusterNetwork, true); err != nil {
		return err
	}
	return w.watchOne(mgr, ctr, commonnetwork.NetworkGVK, indexer.IndexFieldVMByNetwork, false)
}

func (w *NetworkWatcher) watchOne(mgr manager.Manager, ctr controller.Controller, gvk schema.GroupVersionKind, indexField string, clusterScoped bool) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			obj,
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, o *unstructured.Unstructured) []reconcile.Request {
				return w.enqueueVMsReferencingNetwork(ctx, o, indexField, clusterScoped)
			}),
			predicate.TypedFuncs[*unstructured.Unstructured]{
				CreateFunc: func(e event.TypedCreateEvent[*unstructured.Unstructured]) bool { return true },
				DeleteFunc: func(e event.TypedDeleteEvent[*unstructured.Unstructured]) bool { return true },
				UpdateFunc: func(e event.TypedUpdateEvent[*unstructured.Unstructured]) bool {
					return readyConditionStatus(e.ObjectOld) != readyConditionStatus(e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on %s: %w", gvk.Kind, err)
	}
	return nil
}

func (w *NetworkWatcher) enqueueVMsReferencingNetwork(ctx context.Context, obj *unstructured.Unstructured, indexField string, clusterScoped bool) []reconcile.Request {
	var vms v1alpha2.VirtualMachineList
	listOpts := []client.ListOption{client.MatchingFields{indexField: obj.GetName()}}
	if !clusterScoped {
		listOpts = append(listOpts, client.InNamespace(obj.GetNamespace()))
	}
	if err := w.client.List(ctx, &vms, listOpts...); err != nil {
		log.Default().Error(fmt.Sprintf("network watcher: failed to list VMs: %s", err))
		return nil
	}

	requests := make([]reconcile.Request, 0, len(vms.Items))
	for _, vm := range vms.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace},
		})
	}
	return requests
}

func readyConditionStatus(obj *unstructured.Unstructured) string {
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return ""
	}
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _, _ := unstructured.NestedString(m, "type")
		if t != "Ready" {
			continue
		}
		s, _, _ := unstructured.NestedString(m, "status")
		return s
	}
	return ""
}
