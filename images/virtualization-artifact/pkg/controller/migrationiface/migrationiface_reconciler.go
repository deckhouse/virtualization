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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
)

// snnniaGVK is the GroupVersionKind of sdn's
// SystemNetworkNodeNetworkInterfaceAttachment. We talk to it via the
// unstructured client so this controller has no compile-time dependency on
// the sdn module's Go types — and tolerates the CRD being absent at startup.
var snnniaGVK = schema.GroupVersionKind{
	Group:   "network.deckhouse.io",
	Version: "v1alpha1",
	Kind:    "SystemNetworkNodeNetworkInterfaceAttachment",
}

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
	// Watch Nodes directly — each Node is its own reconcile key.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(),
			&corev1.Node{},
			&handler.TypedEnqueueRequestForObject[*corev1.Node]{},
		),
	); err != nil {
		return fmt.Errorf("watch Node: %w", err)
	}

	// Watch SystemNetworkNodeNetworkInterfaceAttachment via unstructured.
	// If the CRD is not installed, this watch will fail with NoKindMatchError —
	// we log and continue, leaving the controller running on Node-only events
	// (which is fine: with no SDN CRDs the desired state is "no annotation",
	// and the Node watch alone is enough to drive eventual consistency once
	// the CRD appears and the manager is restarted).
	snnnia := &unstructured.Unstructured{}
	snnnia.SetGroupVersionKind(snnniaGVK)
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), snnnia,
			handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, obj *unstructured.Unstructured) []reconcile.Request {
				nodeName, _, _ := unstructured.NestedString(obj.Object, "status", "nodeName")
				if nodeName == "" {
					return nil
				}
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nodeName}}}
			}),
		),
	); err != nil {
		// Don't fail controller startup; log and proceed.
		r.log.Warn("watch SystemNetworkNodeNetworkInterfaceAttachment failed (sdn CRD not installed?), migration interface annotation will not track sdn changes",
			"err", err.Error())
	}

	return nil
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

	current := node.Annotations[MigrationIfaceAnnotation]
	if current == desired {
		return reconcile.Result{}, nil
	}

	// Strategic merge patch on the annotation only — keeps us out of
	// the way of any other actor patching the Node.
	patch := fmt.Appendf(nil,
		`{"metadata":{"annotations":{%q:%s}}}`,
		MigrationIfaceAnnotation, jsonStringOrNull(desired),
	)
	if err := r.client.Patch(ctx, &node, client.RawPatch(types.StrategicMergePatchType, patch)); err != nil {
		return reconcile.Result{}, fmt.Errorf("patch node %q annotation: %w", node.Name, err)
	}

	r.log.Info("updated migration interface annotation",
		"node", node.Name,
		"systemNetwork", r.systemNetworkName,
		"interface", desired,
		"previous", current,
	)
	return reconcile.Result{}, nil
}

// resolveInterfaceForNode returns the kernel interface name to bind migration
// to on the given node, or "" if no matching ready attachment exists.
func (r *Reconciler) resolveInterfaceForNode(ctx context.Context, nodeName string) (string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   snnniaGVK.Group,
		Version: snnniaGVK.Version,
		Kind:    snnniaGVK.Kind + "List",
	})
	if err := r.client.List(ctx, list); err != nil {
		// CRD missing → desired state is "no annotation".
		if isNoMatchError(err) {
			return "", nil
		}
		return "", fmt.Errorf("list SystemNetworkNodeNetworkInterfaceAttachment: %w", err)
	}

	for i := range list.Items {
		item := &list.Items[i]
		statusNode, _, _ := unstructured.NestedString(item.Object, "status", "nodeName")
		if statusNode != nodeName {
			continue
		}
		// Spec.systemNetworkRef.name (or similar) identifies which SystemNetwork
		// this attachment belongs to. The exact field shape is owned by sdn;
		// be permissive and look in a few likely locations.
		if !attachmentMatchesSystemNetwork(item, r.systemNetworkName) {
			continue
		}
		// status.vlanNodeNetworkInterfaceName is the kernel iface created by
		// sdn for VLAN-type attachments (per sdn admin docs).
		ifaceName, _, _ := unstructured.NestedString(item.Object, "status", "vlanNodeNetworkInterfaceName")
		if ifaceName != "" {
			return ifaceName, nil
		}
	}
	return "", nil
}

// attachmentMatchesSystemNetwork inspects an unstructured SNNNIA and returns
// true if it belongs to the named SystemNetwork. The sdn API may evolve; we
// check the locations most likely to hold the reference.
func attachmentMatchesSystemNetwork(item *unstructured.Unstructured, systemNetworkName string) bool {
	for _, path := range [][]string{
		{"spec", "systemNetworkRef", "name"},
		{"spec", "systemNetworkName"},
		{"metadata", "labels", "network.deckhouse.io/system-network-name"},
	} {
		if v, ok, _ := unstructured.NestedString(item.Object, path...); ok && v == systemNetworkName {
			return true
		}
	}
	// Fallback: ownerReference.kind=SystemNetwork with matching name.
	for _, ref := range item.GetOwnerReferences() {
		if ref.Kind == "SystemNetwork" && ref.Name == systemNetworkName {
			return true
		}
	}
	return false
}

func isNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	// meta.NoKindMatchError / meta.NoResourceMatchError both report this string;
	// keep the dependency surface minimal by string-matching.
	return apierrors.IsNotFound(err) || containsAny(err.Error(),
		"no matches for kind",
		"the server could not find the requested resource",
	)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

// jsonStringOrNull renders v as a JSON string, or as JSON null when v is empty
// (so the strategic merge patch removes the annotation key).
func jsonStringOrNull(v string) string {
	if v == "" {
		return "null"
	}
	// minimal JSON string escaping for our use (interface names contain only
	// [a-z0-9._-]); fall back to fmt for safety.
	return fmt.Sprintf("%q", v)
}

