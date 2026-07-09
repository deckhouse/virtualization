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

package internal

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameIPAddressHandler = "IPAddressHandler"

// NewIPAddressHandler creates a handler that manages SDN IPAddress resources
// for additional network interfaces of the VM in auto-mode (when the user does
// not specify ipAddressName). This is analogous to IPAMHandler (vmip) for the
// Main network and MACHandler for MAC addresses.
//
// The handler does not set a condition: readiness is aggregated into
// NetworkReady by the NetworkInterfaceHandler, which checks the allocated
// state of the IPAddress resources.
func NewIPAddressHandler(cl client.Client) *IPAddressHandler {
	return &IPAddressHandler{client: cl}
}

type IPAddressHandler struct {
	client client.Client
}

func (h *IPAddressHandler) Name() string {
	return nameIPAddressHandler
}

func (h *IPAddressHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameIPAddressHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	vm := s.VirtualMachine().Changed()

	if isDeletion(vm) {
		return reconcile.Result{}, nil
	}

	// Nothing to do if there are no additional networks.
	if hasOnlyDefaultNetwork(vm) {
		return reconcile.Result{}, nil
	}

	// Ensure auto-IPAddress for additional networks with a pool.
	for _, ns := range vm.Spec.Networks {
		if ns.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		// Static mode: user manages the IPAddress, skip.
		if ns.IPAddressName != "" {
			continue
		}

		// Only create an auto-IPAddress if the network has a pool (IPAM).
		hasPool, err := commonnetwork.HasIPAM(ctx, h.client, vm.Namespace, ns)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("check IPAM for %s: %w", commonnetwork.SpecKey(ns), err)
		}
		if !hasPool {
			continue
		}

		// Ensure the SDN IPAddress exists (idempotent: finds by label+networkRef).
		name, err := commonnetwork.EnsureSDNIPAddress(ctx, h.client, vm, ns)
		if err != nil {
			if apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) || apierrors.IsConflict(err) || apierrors.IsServiceUnavailable(err) {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, fmt.Errorf("ensure SDN IPAddress for %s: %w", commonnetwork.SpecKey(ns), err)
		}
		log.Debug("Ensured SDN IPAddress for additional network", "network", commonnetwork.SpecKey(ns), "ipAddress", name)
	}

	// Garbage-collect auto-IPAddress for removed networks.

	if err := h.cleanupOrphanedIPAddresses(ctx, vm); err != nil {
		return reconcile.Result{}, fmt.Errorf("cleanup orphaned SDN IPAddress: %w", err)
	}

	return reconcile.Result{}, nil
}

// cleanupOrphanedIPAddresses removes auto-IPAddress resources owned by the VM
// whose referenced network is no longer present in vm.Spec.Networks.
func (h *IPAddressHandler) cleanupOrphanedIPAddresses(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	// Build a set of (type, name) for additional networks in spec (with pool, auto).
	wanted := make(map[string]struct{})
	for _, ns := range vm.Spec.Networks {
		if ns.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		// Static mode: user-owned, never GC.
		if ns.IPAddressName != "" {
			continue
		}
		wanted[commonnetwork.SpecKey(ns)] = struct{}{}
	}

	// List IPAddress owned by this VM (by label).
	list, err := commonnetwork.ListSDNIPAddressesForVM(ctx, h.client, vm)
	if err != nil {
		return err
	}
	for _, ip := range list {
		// Skip if the network is still wanted.
		key := ip.NetworkRefKind + "/" + ip.NetworkRefName
		if _, ok := wanted[key]; ok {
			continue
		}
		// This IPAddress is orphaned (network removed from spec). Delete it.
		if err := commonnetwork.DeleteSDNIPAddressByName(ctx, h.client, vm.Namespace, ip.Name); err != nil {
			return fmt.Errorf("delete orphaned SDN IPAddress %s: %w", ip.Name, err)
		}
	}
	return nil
}
