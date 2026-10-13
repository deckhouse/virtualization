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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// SDNIPAddressWatcher requeues the vmips an IPAddress of the Main network concerns:
// the one it is the mirror of, and those whose address it takes. The vmip becomes
// Bound only after its mirror is Allocated, and another IPAddress on its address
// keeps the mirror from being allocated.
type SDNIPAddressWatcher struct {
	client client.Client
	logger *log.Logger
}

func NewSDNIPAddressWatcher(client client.Client) *SDNIPAddressWatcher {
	return &SDNIPAddressWatcher{
		client: client,
		logger: log.Default().With("watcher", "sdnipaddress"),
	}
}

func (w SDNIPAddressWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	// Without the sdn module the IPAddress kind does not exist, and the mirror step
	// does nothing: there is no ClusterNetwork main either.
	if !featuregates.Default().Enabled(featuregates.SDN) {
		return nil
	}
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&sdnapi.IPAddress{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueVMIPsOfIPAddress),
			predicate.TypedFuncs[*sdnapi.IPAddress]{
				CreateFunc:  func(e event.TypedCreateEvent[*sdnapi.IPAddress]) bool { return true },
				DeleteFunc:  func(e event.TypedDeleteEvent[*sdnapi.IPAddress]) bool { return true },
				GenericFunc: func(e event.TypedGenericEvent[*sdnapi.IPAddress]) bool { return false },
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on SDN IPAddress: %w", err)
	}

	return nil
}

// enqueueVMIPsOfIPAddress enqueues the vmips of the namespace the IPAddress is the
// mirror of or takes the address of.
func (w SDNIPAddressWatcher) enqueueVMIPsOfIPAddress(ctx context.Context, ipa *sdnapi.IPAddress) []reconcile.Request {
	address := commonnetwork.AddressOfIPAddress(ipa)
	if address == "" && !commonnetwork.IsMirror(ipa) {
		return nil
	}

	var vmips v1alpha2.VirtualMachineIPAddressList
	if err := w.client.List(ctx, &vmips, client.InNamespace(ipa.GetNamespace())); err != nil {
		w.logger.Error("failed to list vmips", "err", err)
		return nil
	}
	vmipNamedByLease := w.vmipNamedByLease(ctx, ipa.GetNamespace(), address)

	var requests []reconcile.Request
	for i := range vmips.Items {
		vmip := &vmips.Items[i]
		if isMirrorOfVMIP(ipa, vmip) || takesVMIPAddress(vmip, address, vmipNamedByLease) {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      vmip.Name,
				Namespace: vmip.Namespace,
			}})
		}
	}
	return requests
}

// isMirrorOfVMIP also matches by the name: the mirror is named after its vmip.
func isMirrorOfVMIP(ipa *sdnapi.IPAddress, vmip *v1alpha2.VirtualMachineIPAddress) bool {
	return commonnetwork.IsMirrorOf(ipa, vmip) || ipa.GetName() == vmip.Name
}

// takesVMIPAddress also matches the vmip the lease of the address names: the lease
// comes before the address reaches the status of the vmip.
func takesVMIPAddress(vmip *v1alpha2.VirtualMachineIPAddress, address, vmipNamedByLease string) bool {
	if address == "" {
		return false
	}
	return vmip.Status.Address == address || vmip.Spec.StaticIP == address || vmip.Name == vmipNamedByLease
}

func (w SDNIPAddressWatcher) vmipNamedByLease(ctx context.Context, namespace, address string) string {
	if address == "" {
		return ""
	}
	lease := &v1alpha2.VirtualMachineIPAddressLease{}
	err := w.client.Get(ctx, types.NamespacedName{Name: ip.IPToLeaseName(address)}, lease)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			w.logger.Error("failed to get the lease of the address", "address", address, "err", err)
		}
		return ""
	}
	if ref := lease.Spec.VirtualMachineIPAddressRef; ref != nil && ref.Namespace == namespace {
		return ref.Name
	}
	return ""
}
