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

package step

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

// MirrorStep keeps the mirror of the vmip address. While the sdn module serves the
// Main network, a vmip becomes Bound only after its mirror is Allocated. Runs before BindStep.
type MirrorStep struct {
	lease    *v1alpha2.VirtualMachineIPAddressLease
	client   client.Client
	cb       *conditions.ConditionBuilder
	recorder eventrecord.EventRecorderLogger
}

// mirrorTerminatingRetry paces the wait for the previous mirror to go away.
const mirrorTerminatingRetry = 5 * time.Second

// mirrorNameTakenRetry paces the wait for the resource with the mirror name
// to go away: only a change outside this vmip can unblock it.
const mirrorNameTakenRetry = 30 * time.Second

func NewMirrorStep(
	lease *v1alpha2.VirtualMachineIPAddressLease,
	client client.Client,
	cb *conditions.ConditionBuilder,
	recorder eventrecord.EventRecorderLogger,
) *MirrorStep {
	return &MirrorStep{
		lease:    lease,
		client:   client,
		cb:       cb,
		recorder: recorder,
	}
}

// isVMMovedToIPAddress: the owner of vm, created together with the vmip, named an
// IPAddress in spec.networks[].ipAddressName. The vmip stays, so that the VM gets its
// address back once the owner removes the name.
func (s MirrorStep) isVMMovedToIPAddress(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress, vm *v1alpha2.VirtualMachine) (bool, error) {
	if vm == nil || vmip.Labels[annotations.LabelVirtualMachineUID] != string(vm.UID) {
		return false, nil
	}
	usesVMIP, err := network.UsesVMIP(ctx, s.client, vm)
	if err != nil || usesVMIP {
		return false, err
	}
	return vm.Status.IPAddress != "" && vm.Status.IPAddress != vmip.Status.Address, nil
}

func (s MirrorStep) Take(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (*reconcile.Result, error) {
	address := vmip.Status.Address
	if address == "" && s.lease != nil {
		address = ip.LeaseNameToIP(s.lease.Name)
	}
	if address == "" {
		// Nothing allocated yet; the mirror appears after the lease is created.
		return nil, nil
	}

	// A cluster where the sdn module does not serve the Main network goes through
	// this step untouched: there is nothing to mirror the address into.
	mainIPAMAvailable, err := network.MainIPAMAvailable(ctx, s.client)
	if err != nil {
		return nil, fmt.Errorf("check whether the sdn module serves the Main network: %w", err)
	}
	if !mainIPAMAvailable {
		return nil, nil
	}

	// The mirror is matched to the vmip by status.address, so set it before BindStep does.
	vmip.Status.Address = address

	result, err := s.ensureMirror(ctx, vmip)
	if result == nil && err == nil {
		return nil, nil
	}
	// A machine may run on the address of a Bound vmip: losing Bound would keep it from
	// starting again. The message stays and says what is wrong with the mirror.
	boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, vmip.Status.Conditions)
	if boundCondition.Status == metav1.ConditionTrue && intsvc.HasReference(vmip, s.lease) {
		s.cb.Status(metav1.ConditionTrue).Reason(vmipcondition.Bound)
	}
	return result, err
}

// ensureMirror creates the mirror of the address; a nil result lets the chain go on.
func (s MirrorStep) ensureMirror(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (*reconcile.Result, error) {
	vm, err := network.GetVMOfVMIP(ctx, s.client, vmip)
	if err != nil {
		return nil, err
	}
	if vmip.GetDeletionTimestamp() != nil && !isAttachedTo(vmip, vm) {
		return nil, nil
	}
	if err := network.ReleaseTerminatingMirror(ctx, s.client, vmip, vm); err != nil {
		return nil, err
	}
	vmMovedToIPAddress, err := s.isVMMovedToIPAddress(ctx, vmip, vm)
	if err != nil {
		return nil, err
	}
	if vmip.GetDeletionTimestamp() != nil && vmMovedToIPAddress {
		return nil, nil
	}

	ipa, err := network.EnsureVMIPAddressTaken(ctx, s.client, vmip, network.MirrorOwnerRef(vmip, vm, vmMovedToIPAddress))
	if err != nil {
		return setConditionFromMirrorError(s.cb, err)
	}
	if ipa == nil {
		return setMirrorNameTakenCondition(s.cb, vmip.Name), nil
	}
	if vmMovedToIPAddress {
		// The VM does not run on the address of this vmip: its Bound does not wait
		// for the mirror.
		return nil, nil
	}

	switch {
	case network.IPAddressAllocated(ipa):
		// Confirmed by the sdn module: continue the chain (BindStep decides Bound).
		return nil, nil
	case !network.IPAddressTakenByAnother(ipa):
		setLeaseNotReadyCondition(s.cb, fmt.Sprintf("Waiting for the sdn module to allocate the address to the IPAddress %q.", ipa.GetName()))
		return &reconcile.Result{}, nil
	case s.canTakeAnotherAddress(vmip):
		return s.takeAnotherAddress(ctx, vmip, ipa)
	default:
		setLeaseNotReadyCondition(s.cb, fmt.Sprintf("The sdn module refused the address %s for the IPAddress %q: %s. Delete the IPAddress that already has this address, or give this VirtualMachineIPAddress another address.", vmip.Status.Address, ipa.GetName(), ipAddressRefusalMessage(ipa)))
		return &reconcile.Result{}, nil
	}
}

// A deleting vmip mirrors only for the VM running on it, so the mirror never becomes an orphan.
// Not by Attached: AttachedHandler runs after this step.
func isAttachedTo(vmip *v1alpha2.VirtualMachineIPAddress, vm *v1alpha2.VirtualMachine) bool {
	return vm != nil && vm.Status.VirtualMachineIPAddress == vmip.Name
}

func setLeaseNotReadyCondition(cb *conditions.ConditionBuilder, message string) {
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
		Message(message)
}

// setConditionFromMirrorError turns a mirror creation failure into the result to reconcile
// with, or passes the error on when there is nothing to report.
func setConditionFromMirrorError(cb *conditions.ConditionBuilder, err error) (*reconcile.Result, error) {
	switch {
	case errors.Is(err, network.ErrIPAddressTerminating):
		setLeaseNotReadyCondition(cb, "Waiting for the previous IPAddress of this address to be deleted.")
		return &reconcile.Result{RequeueAfter: mirrorTerminatingRetry}, nil
	case common.ErrNamespaceTerminating(err):
		// The namespace is being torn down, so the vmip is on its way out with it and
		// admission rejects the mirror for good: there is nothing to retry.
		setLeaseNotReadyCondition(cb, "Namespace is terminating: the IPAddress that keeps this address in the sdn module cannot be created.")
		return &reconcile.Result{}, nil
	default:
		return nil, fmt.Errorf("ensure the mirror IPAddress: %w", err)
	}
}

// setMirrorNameTakenCondition reports the mirror name taken by an unrelated IPAddress: only
// a change outside this vmip can unblock it.
func setMirrorNameTakenCondition(cb *conditions.ConditionBuilder, name string) *reconcile.Result {
	setLeaseNotReadyCondition(cb, fmt.Sprintf("Cannot create the IPAddress %q that keeps this address in the sdn module: an IPAddress of that name, not created for this VirtualMachineIPAddress, exists. Delete it.", name))
	return &reconcile.Result{RequeueAfter: mirrorNameTakenRetry}
}

// canTakeAnotherAddress reports whether the vmip may give up an address the sdn
// module has handed out to another IPAddress. Only an Auto vmip that is not Bound
// may: a Bound or a Static one keeps it, because changing the address of a running
// machine is not an option.
func (s MirrorStep) canTakeAnotherAddress(vmip *v1alpha2.VirtualMachineIPAddress) bool {
	boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, vmip.Status.Conditions)
	return vmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeAuto &&
		boundCondition.Status != metav1.ConditionTrue && s.lease != nil
}

// takeAnotherAddress releases the lease of the address the sdn module has handed out
// to another IPAddress, together with the mirror of this vmip on it, so that the
// vmip gets another address. A resource of somebody else stays.
func (s MirrorStep) takeAnotherAddress(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress, ipa *sdnapi.IPAddress) (*reconcile.Result, error) {
	msg := fmt.Sprintf("The sdn module refused the address %s (%s): picking another address for this VirtualMachineIPAddress.", vmip.Status.Address, ipAddressRefusalMessage(ipa))
	s.recorder.Event(vmip, corev1.EventTypeWarning, vmipcondition.VirtualMachineIPAddressLeaseNotReady.String(), msg)

	if network.IsMirrorOf(ipa, vmip) {
		if err := network.DeleteSDNIPAddressByName(ctx, s.client, vmip.Namespace, ipa.GetName()); err != nil {
			return nil, fmt.Errorf("delete the conflicting mirror IPAddress %q: %w", ipa.GetName(), err)
		}
	}
	if err := s.client.Delete(ctx, s.lease); err != nil && !k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("release the conflicting lease %q: %w", s.lease.Name, err)
	}
	vmip.Status.Address = ""
	setLeaseNotReadyCondition(s.cb, msg)
	return &reconcile.Result{}, nil
}

// ipAddressRefusalMessage returns why the sdn module refused the address to the IPAddress.
func ipAddressRefusalMessage(ipa *sdnapi.IPAddress) string {
	cond := meta.FindStatusCondition(ipa.Status.Conditions, sdnapi.ConditionAllocated)
	if cond == nil || cond.Status == metav1.ConditionTrue || cond.Message == "" {
		return "the address is already in use"
	}
	return strings.TrimSuffix(cond.Message, ".")
}
