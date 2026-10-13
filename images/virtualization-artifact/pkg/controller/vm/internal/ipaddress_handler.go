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
	"errors"
	"fmt"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameIPAddressHandler = "IPAddressHandler"

// ipAddressTerminatingRetry paces the wait for the previous address of the
// virtual machine to go away.
const ipAddressTerminatingRetry = 5 * time.Second

// addressInUseRetry paces the retry when another resource uses the address: only
// the user can resolve that.
const addressInUseRetry = 10 * time.Minute

// NewIPAddressHandler creates the handler of the IPAddresses the VM gets without
// ipAddressName, for the Main and the additional networks. Readiness is reported
// by IPAMHandler and NetworkInterfaceHandler.
func NewIPAddressHandler(cl client.Client, recorder eventrecord.EventRecorderLogger) *IPAddressHandler {
	return &IPAddressHandler{client: cl, recorder: recorder}
}

type IPAddressHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
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
		// The addresses are owned by the virtual machine and go with it.
		return reconcile.Result{}, nil
	}

	mainInterfaceSpecEnabled, err := s.MainInterfaceSpecEnabled(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	// A machine on the Main network alone whose pod gets no Main interface spec takes
	// its address from a vmip and has no IPAddress to keep.
	if hasOnlyDefaultNetwork(vm) && !mainInterfaceSpecEnabled {
		return reconcile.Result{}, nil
	}

	ipAddresses, err := commonnetwork.ListSDNIPAddressesOfVM(ctx, h.client, vm)
	if err != nil {
		if apimeta.IsNoMatchError(err) {
			// The IPAMHandler reports it in the condition.
			log.Debug("The IPAddress kind is not registered, nothing to ensure")
			return reconcile.Result{RequeueAfter: ipAddressKindRetryInterval}, nil
		}
		return reconcile.Result{}, err
	}

	if err := commonnetwork.ReleaseTerminatingSDNIPAddresses(ctx, h.client, vm, ipAddresses); err != nil {
		return reconcile.Result{}, fmt.Errorf("release terminating SDN IPAddress: %w", err)
	}

	usesVMIP, err := s.UsesVMIP(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	// The pod of a machine on a vmip gets the mirror of the vmip as its Main IPAddress.
	onMirrorOfVMIP := usesVMIP && mainInterfaceSpecEnabled

	// The result for the Main network is returned after the additional networks, so a
	// wait for the Main address does not delay them.
	var mainResult reconcile.Result
	switch {
	case onMirrorOfVMIP:
		mainResult, err = h.labelMirrorOfVMIP(ctx, s, vm)
	case !usesVMIP:
		mainResult, err = h.ensureMainIPAddress(ctx, vm, ipAddresses, log)
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	// Ensure auto-IPAddress for additional networks with a pool.
	for _, netSpec := range vm.Spec.Networks {
		if netSpec.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		// Static mode: user manages the IPAddress, skip.
		if netSpec.IPAddressName != "" {
			continue
		}

		// Only create an auto-IPAddress if the network has a pool (IPAM).
		hasPool, err := commonnetwork.HasIPAM(ctx, h.client, vm.Namespace, netSpec)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("check IPAM for %s: %w", commonnetwork.SpecKey(netSpec), err)
		}
		if !hasPool {
			continue
		}

		// Ensure the SDN IPAddress exists (idempotent: finds by label+networkRef).
		name, err := commonnetwork.EnsureSDNIPAddress(ctx, h.client, vm, netSpec, ipAddresses)
		if err != nil {
			if errors.Is(err, commonnetwork.ErrIPAddressTerminating) {
				log.Debug("The IPAddress of the additional network is still terminating, waiting",
					"network", commonnetwork.SpecKey(netSpec))
				return reconcile.Result{RequeueAfter: ipAddressTerminatingRetry}, nil
			}
			if apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) || apierrors.IsConflict(err) || apierrors.IsServiceUnavailable(err) {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, fmt.Errorf("ensure SDN IPAddress for %s: %w", commonnetwork.SpecKey(netSpec), err)
		}
		log.Debug("Ensured SDN IPAddress for additional network", "network", commonnetwork.SpecKey(netSpec), "ipAddress", name)
	}

	// Garbage-collect auto-IPAddress for removed networks.

	if onMirrorOfVMIP {
		// labelMirrorOfVMIP has moved the label of the VM between the mirrors.
		if ipAddresses, err = commonnetwork.ListSDNIPAddressesOfVM(ctx, h.client, vm); err != nil {
			return reconcile.Result{}, err
		}
	}
	if err := h.cleanupOrphanedIPAddresses(ctx, vm, ipAddresses, mainInterfaceSpecEnabled); err != nil {
		return reconcile.Result{}, fmt.Errorf("cleanup orphaned SDN IPAddress: %w", err)
	}

	return mainResult, nil
}

// labelMirrorOfVMIP sets the label of the machine on the mirror of its vmip and takes
// it off the mirrors of its previous vmips: the Main interface spec finds the
// IPAddress by the label. The vmip controller creates the mirror; an IPAddress created
// here would take its address or allocate another one. Another resource taking the
// address is reported in an event.
func (h *IPAddressHandler) labelMirrorOfVMIP(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	vmip, err := s.IPAddress(ctx)
	if err != nil || vmip == nil || vmip.Status.Address == "" {
		return reconcile.Result{}, err
	}
	ipa, err := commonnetwork.GetIPAddressTakingVMIPAddress(ctx, h.client, vmip)
	if err != nil || ipa == nil || ipa.GetDeletionTimestamp() != nil || !commonnetwork.IPAddressAllocated(ipa) {
		return reconcile.Result{}, err
	}

	switch {
	case ipa.GetLabels()[annotations.LabelVirtualMachineUID] == string(vm.GetUID()):
		// Already this VM's own resource, whether it was created as a mirror or
		// as the Main IPAddress of the VM itself.
	case commonnetwork.IsMirrorOf(ipa, vmip):
		if err := commonnetwork.LabelMirrorWithVM(ctx, h.client, vm, ipa); err != nil {
			return reconcile.Result{}, fmt.Errorf("label the mirror IPAddress %q: %w", ipa.GetName(), err)
		}
	default:
		h.recorder.Eventf(vm, corev1.EventTypeWarning, v1alpha2.ReasonMainIPAMMigrationBlocked,
			"The address %s of the virtual machine cannot be moved to the sdn module: the IPAddress %q, not created for the VirtualMachineIPAddress %q, already uses it. Delete that IPAddress, or reference it in spec.networks[].ipAddressName of the Main network and restart the virtual machine.",
			vmip.Status.Address, ipa.GetName(), vmip.Name)
		return reconcile.Result{RequeueAfter: addressInUseRetry}, nil
	}
	return reconcile.Result{}, commonnetwork.ReleaseMirrorsOfPreviousVMIPs(ctx, h.client, vm, vmip)
}

// ensureMainIPAddress keeps the IPAddress of the Main network a VM without a vmip
// gets when it names none in ipAddressName.
func (h *IPAddressHandler) ensureMainIPAddress(ctx context.Context, vm *v1alpha2.VirtualMachine, ipAddresses []sdnapi.IPAddress, log *slog.Logger) (reconcile.Result, error) {
	mainSpec := commonnetwork.MainNetworkSpecOrImplicit(vm)
	if mainSpec == nil || mainSpec.IPAddressName != "" {
		return reconcile.Result{}, nil
	}
	name, err := commonnetwork.EnsureMainSDNIPAddress(ctx, h.client, vm, ipAddresses)
	switch {
	case errors.Is(err, commonnetwork.ErrIPAddressTerminating):
		log.Debug("The Main network IPAddress is still terminating, waiting")
		return reconcile.Result{RequeueAfter: ipAddressTerminatingRetry}, nil
	case err != nil:
		return reconcile.Result{}, fmt.Errorf("ensure SDN IPAddress for the Main network: %w", err)
	}
	log.Debug("Ensured SDN IPAddress for the Main network", "ipAddress", name)
	return reconcile.Result{}, nil
}

// cleanupOrphanedIPAddresses removes auto-IPAddress resources owned by the VM
// whose referenced network is no longer present in vm.Spec.Networks, and the
// duplicates of a network the VM does not use.
func (h *IPAddressHandler) cleanupOrphanedIPAddresses(ctx context.Context, vm *v1alpha2.VirtualMachine, ipAddresses []sdnapi.IPAddress, mainInterfaceSpecEnabled bool) error {
	// Networks are keyed by the networkRef (not SpecKey) so the Main network maps to
	// the same ClusterNetwork/main its IPAddress carries.
	wanted := make(map[sdnapi.NetworkRef]struct{})
	if mainInterfaceSpecEnabled {
		if mainSpec := commonnetwork.MainNetworkSpecOrImplicit(vm); mainSpec != nil && mainSpec.IPAddressName == "" {
			wanted[commonnetwork.SDNNetworkRef(*mainSpec)] = struct{}{}
		}
	}
	for _, netSpec := range vm.Spec.Networks {
		if netSpec.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		// Static mode: user-owned, never GC.
		if netSpec.IPAddressName != "" {
			continue
		}
		wanted[commonnetwork.SDNNetworkRef(netSpec)] = struct{}{}
	}

	// Names referenced by the spec (static mode) are user-managed regardless of
	// labels and must never be garbage-collected.
	referenced := make(map[string]struct{})
	for _, netSpec := range commonnetwork.WithImplicitMain(vm.Spec.Networks) {
		if netSpec.IPAddressName != "" {
			referenced[netSpec.IPAddressName] = struct{}{}
		}
	}

	byNetwork := make(map[sdnapi.NetworkRef][]sdnapi.IPAddress)
	for i := range ipAddresses {
		key := ipAddresses[i].Spec.NetworkRef
		byNetwork[key] = append(byNetwork[key], ipAddresses[i])
	}
	for i := range ipAddresses {
		ipa := &ipAddresses[i]
		if _, ok := referenced[ipa.Name]; ok {
			continue
		}
		if commonnetwork.IsMainNetworkIPAddress(ipa) {
			// Without the Main interface spec the address comes from a vmip, and the
			// IPAddresses of the Main network are not this handler's to collect.
			if !mainInterfaceSpecEnabled {
				continue
			}
			// The address the machine runs on is never orphaned: repinning takes a
			// restart, and until then the route to the machine is programmed from it.
			if commonnetwork.IPAddressInUse(vm, ipa) {
				continue
			}
			// A mirror goes with its vmip: the vmip controller deletes it. The mirror
			// of a new vmip is not used until the restart, and the vmip controller
			// would recreate it after every deletion.
			if commonnetwork.IsMirror(ipa) {
				continue
			}
		}
		// A wanted network keeps the IPAddress the VM uses; another one is a duplicate
		// the generated name let in.
		key := ipa.Spec.NetworkRef
		if _, ok := wanted[key]; ok {
			preferred := commonnetwork.PreferredSDNIPAddress(vm, byNetwork[key])
			if ipa.Name == preferred.Name || ipa.DeletionTimestamp != nil || commonnetwork.IPAddressInUse(vm, ipa) {
				continue
			}
		}
		if err := commonnetwork.DeleteSDNIPAddressByName(ctx, h.client, vm.Namespace, ipa.Name); err != nil {
			return fmt.Errorf("delete orphaned SDN IPAddress %s: %w", ipa.Name, err)
		}
	}
	return nil
}
