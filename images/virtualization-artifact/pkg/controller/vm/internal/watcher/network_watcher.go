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
	"k8s.io/component-base/featuregate"
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
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// NewNetworkWatcher enqueues VMs that reference a Network or ClusterNetwork
// when that network's Ready condition changes.
func NewNetworkWatcher(c client.Client, featureGate featuregate.FeatureGate) *NetworkWatcher {
	return &NetworkWatcher{client: c, featureGate: featureGate}
}

type NetworkWatcher struct {
	client      client.Client
	featureGate featuregate.FeatureGate
}

func (w *NetworkWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if !w.featureGate.Enabled(featuregates.SDN) {
		return nil
	}
	if err := w.watchOne(mgr, ctr, commonnetwork.ClusterNetworkGVK, indexer.IndexFieldVMByClusterNetwork, true); err != nil {
		return err
	}
	if err := w.watchOne(mgr, ctr, commonnetwork.NetworkGVK, indexer.IndexFieldVMByNetwork, false); err != nil {
		return err
	}
	return w.watchIPAddress(mgr, ctr)
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

// watchIPAddress watches SDN IPAddress resources owned by VMs and enqueues
// the owning VM when the IPAddress status changes (e.g. NoFreeIPAddress ->
// IPAddressAllocated when the pool is replenished). This enables reactive
// reconciliation without periodic requeue when a previously exhausted pool
// gets free addresses.
func (w *NetworkWatcher) watchIPAddress(mgr manager.Manager, ctr controller.Controller) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(commonnetwork.IPAddressGVK)

	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			obj,
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, o *unstructured.Unstructured) []reconcile.Request {
				return w.enqueueVMOwningIPAddress(ctx, o)
			}),
			predicate.TypedFuncs[*unstructured.Unstructured]{
				CreateFunc: func(e event.TypedCreateEvent[*unstructured.Unstructured]) bool { return true },
				DeleteFunc: func(e event.TypedDeleteEvent[*unstructured.Unstructured]) bool { return true },
				UpdateFunc: func(e event.TypedUpdateEvent[*unstructured.Unstructured]) bool {
					return ipAddressAllocatedStatus(e.ObjectOld) != ipAddressAllocatedStatus(e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on IPAddress: %w", err)
	}
	return nil
}

// enqueueVMOwningIPAddress enqueues VMs affected by the given IPAddress:
//   - VMs that own the IPAddress (via ownerReferences with kind VirtualMachine)
//     — this covers auto-IPAddresses created by the controller.
//   - VMs that reference the IPAddress by name in spec.networks[].ipAddressName
//     — this covers static (user-provided) IPAddresses.
func (w *NetworkWatcher) enqueueVMOwningIPAddress(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
	var requests []reconcile.Request
	seen := make(map[types.NamespacedName]struct{})

	// 1. Owner reference (auto-IPAddress created by controller).
	for _, owner := range obj.GetOwnerReferences() {
		if owner.Kind == v1alpha2.VirtualMachineKind {
			nn := types.NamespacedName{Name: owner.Name, Namespace: obj.GetNamespace()}
			if _, ok := seen[nn]; !ok {
				seen[nn] = struct{}{}
				requests = append(requests, reconcile.Request{NamespacedName: nn})
			}
		}
	}

	// 2. Reference by name in vm.spec.networks[].ipAddressName (static IPAddress).
	var vms v1alpha2.VirtualMachineList
	if err := w.client.List(ctx, &vms, client.InNamespace(obj.GetNamespace())); err != nil {
		log.Default().Error(fmt.Sprintf("ipaddress watcher: failed to list VMs: %s", err))
		return requests
	}
	for _, vm := range vms.Items {
		for _, ns := range vm.Spec.Networks {
			if ns.IPAddressName == obj.GetName() {
				nn := types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}
				if _, ok := seen[nn]; !ok {
					seen[nn] = struct{}{}
					requests = append(requests, reconcile.Request{NamespacedName: nn})
				}
				break
			}
		}
	}

	return requests
}

// ipAddressAllocatedStatus returns the Allocated condition status (True/False)
// of an SDN IPAddress, or "" if not found. Used as a change predicate.
func ipAddressAllocatedStatus(obj *unstructured.Unstructured) string {
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
		if t != "Allocated" {
			continue
		}
		s, _, _ := unstructured.NestedString(m, "status")
		return s
	}
	return ""
}

func readyConditionStatus(obj *unstructured.Unstructured) string {
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		log.Default().Error(fmt.Sprintf("network watcher: read status.conditions of %s/%s: %s", obj.GetKind(), obj.GetName(), err))
		return ""
	}
	if !found {
		return ""
	}
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _, err := unstructured.NestedString(m, "type")
		if err != nil {
			log.Default().Error(fmt.Sprintf("network watcher: read condition.type of %s/%s: %s", obj.GetKind(), obj.GetName(), err))
			return ""
		}
		if t != "Ready" {
			continue
		}
		s, _, err := unstructured.NestedString(m, "status")
		if err != nil {
			log.Default().Error(fmt.Sprintf("network watcher: read Ready condition.status of %s/%s: %s", obj.GetKind(), obj.GetName(), err))
			return ""
		}
		return s
	}
	return ""
}
