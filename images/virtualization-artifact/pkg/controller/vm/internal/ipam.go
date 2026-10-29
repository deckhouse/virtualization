/*
Copyright 2024 Flant JSC

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
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

const nameIpamHandler = "IPAMHandler"

type IPAM interface {
	IsBound(vmName string, vmip *v1alpha2.VirtualMachineIPAddress) bool
	CheckIPAddressAvailableForBinding(vmName string, vmip *v1alpha2.VirtualMachineIPAddress) error
	CreateIPAddress(ctx context.Context, vm *v1alpha2.VirtualMachine, client client.Client, staticIP string) error
}

func NewIPAMHandler(ipam IPAM, cl client.Client, recorder eventrecord.EventRecorderLogger) *IPAMHandler {
	return &IPAMHandler{
		ipam:     ipam,
		client:   cl,
		recorder: recorder,
	}
}

type IPAMHandler struct {
	ipam      IPAM
	client    client.Client
	apiReader client.Reader
	recorder  eventrecord.EventRecorderLogger
}

// WithAPIReader lets the handler re-read the address of a received VM past the cache: the address may
// have been created by the previous reconcile and the informer may not have seen it yet.
func (h *IPAMHandler) WithAPIReader(reader client.Reader) *IPAMHandler {
	h.apiReader = reader
	return h
}

func (h *IPAMHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameIpamHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	vm := s.VirtualMachine().Changed()

	if isDeletion(vm) {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeIPAddressReady).
		Status(metav1.ConditionUnknown).
		Reason(conditions.ReasonUnknown).
		Generation(vm.GetGeneration())

	defer func() {
		conditions.SetCondition(cb, &vm.Status.Conditions)
	}()

	if !network.HasMainNetworkStatus(vm.Status.Networks) {
		vm.Status.IPAddress = ""
		vm.Status.VirtualMachineIPAddress = ""
		if err := h.deleteManagedVMIP(ctx, s, vm); err != nil {
			cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonIPAddressNotReady).
				Message(fmt.Sprintf("Failed to delete VirtualMachineIPAddress: %v", err))
			return reconcile.Result{}, err
		}
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonIPAddressReady)
		return reconcile.Result{}, nil
	}

	usesVMIP, err := s.UsesVMIP(ctx)
	if err != nil {
		cb.Reason(vmcondition.ReasonIPAddressNotReady).Message(fmt.Sprintf("Failed to check whether the address of the Main network comes from a VirtualMachineIPAddress or an IPAddress: %v", err))
		return reconcile.Result{}, err
	}
	if !usesVMIP {
		return h.setMainIPAddressStatus(ctx, s, vm, cb, log)
	}

	ipAddress, err := s.IPAddress(ctx)
	if err != nil {
		cb.Reason(vmcondition.ReasonIPAddressNotReady).Message(fmt.Sprintf("Failed to get VirtualMachineIPAddress: %v", err))
		return reconcile.Result{}, err
	}

	// 1. OK: already bound.
	if h.ipam.IsBound(vm.GetName(), ipAddress) {
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonIPAddressReady)
		vm.Status.VirtualMachineIPAddress = ipAddress.GetName()
		if vm.Status.Phase != v1alpha2.MachineRunning && vm.Status.Phase != v1alpha2.MachineStopping {
			vm.Status.IPAddress = ipAddress.Status.Address
		}
		if err := h.checkAddressAssignedInGuest(ctx, s, ipAddress.Status.Address, cb, log); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionFalse)

	// 2. Ip address not found: create if possible or wait for the ip address.
	if ipAddress == nil {
		cb.Reason(vmcondition.ReasonIPAddressNotReady)
		if vm.Spec.VirtualMachineIPAddress != "" {
			log.Info(fmt.Sprintf("The requested VirtualMachineIPAddress %q was not found: waiting for it to appear.", vm.Spec.VirtualMachineIPAddress))
			cb.Message(fmt.Sprintf("The requested VirtualMachineIPAddress %q was not found: waiting for it to appear.", vm.Spec.VirtualMachineIPAddress))
			return reconcile.Result{}, nil
		}
		log.Info("VirtualMachineIPAddress not found: create the new one", slog.String("vmipName", vm.GetName()))
		cb.Message(fmt.Sprintf("VirtualMachineIPAddress %q was not found: it may be in the process of being created.", vm.GetName()))
		receiver, err := loadMigrationReceiver(ctx, h.client, vm)
		if err != nil {
			return reconcile.Result{}, err
		}
		if address := peering.ReceiverIPAddress(receiver); isReceiving(receiver) && address != "" {
			// A VM received from another cluster keeps its address.
			return reconcile.Result{}, h.createReceiverIPAddress(ctx, vm, receiver, address)
		}
		return reconcile.Result{}, h.ipam.CreateIPAddress(ctx, vm, h.client, "")
	}

	// 3. Check if possible to bind virtual machine with the found ip address.
	err = h.ipam.CheckIPAddressAvailableForBinding(vm.GetName(), ipAddress)
	if err != nil {
		log.Info("Ip address is not available to be bound", "err", err, "vmipName", vm.Spec.VirtualMachineIPAddress)
		reason := vmcondition.ReasonIPAddressNotAvailable
		cb.Reason(reason).Message(err.Error())
		h.recorder.Event(vm, corev1.EventTypeWarning, reason.String(), err.Error())
		return reconcile.Result{}, nil
	}

	// 4. Ip address exist and attached to another VirtualMachine
	if ipAddress.Status.VirtualMachine != "" && ipAddress.Status.VirtualMachine != vm.Name {
		msg := fmt.Sprintf("The requested VirtualMachineIPAddress %q is attached to VirtualMachine %q: waiting for it to be released.", vm.Spec.VirtualMachineIPAddress, ipAddress.Status.VirtualMachine)
		log.Info(msg)
		cb.Reason(vmcondition.ReasonIPAddressNotReady).Message(msg)
		return reconcile.Result{}, nil
	}

	// 5. Ip address exists and available for binding with virtual machine: waiting for the ip address.
	log.Info("Waiting for the ip address to be bound to VM", "vmipName", vm.Spec.VirtualMachineIPAddress)
	cb.Reason(vmcondition.ReasonIPAddressNotReady).Message("The IP address is not bound yet: waiting for it to be assigned.")

	return reconcile.Result{}, nil
}

// addressConflictRetryInterval paces the retry of a VM blocked by another VM
// referencing the same IPAddress: nothing links the two, so the block would
// otherwise hold until an unrelated event.
const addressConflictRetryInterval = 30 * time.Second

// ipAddressKindRetryInterval paces the wait for the IPAddress kind to be
// registered again: with it gone there is no watch to wake the reconcile up.
const ipAddressKindRetryInterval = time.Minute

// setMainIPAddressStatus fills the IPAddressReady condition and status.ipAddress from
// the IPAddress of the Main network. The condition gates the KVVM sync, so the VM stays
// Pending until the address is allocated.
func (h *IPAMHandler) setMainIPAddressStatus(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine, cb *conditions.ConditionBuilder, log *slog.Logger) (reconcile.Result, error) {
	mainSpec := network.MainNetworkSpec(vm.Spec.Networks)
	if mainSpec == nil {
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonIPAddressReady)
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonIPAddressNotReady)

	ipa, err := network.GetMainSDNIPAddress(ctx, h.client, vm)
	switch {
	case apimeta.IsNoMatchError(err):
		cb.Message("The IPAddress of the Main network cannot be read: the cluster does not serve the IPAddress kind. The virtual machine cannot be started or migrated until it does; ask the cluster administrator to check the sdn module.")
		return reconcile.Result{RequeueAfter: ipAddressKindRetryInterval}, nil
	case err != nil:
		cb.Message(fmt.Sprintf("Failed to get the IPAddress of the Main network: %v", err))
		return reconcile.Result{}, err
	}
	if ipa == nil {
		if mainSpec.IPAddressName == "" {
			cb.Message("The IPAddress of the Main network was not found: it may be in the process of being created.")
		} else {
			cb.Message(fmt.Sprintf("The requested IPAddress %q was not found: waiting for it to appear.", mainSpec.IPAddressName))
		}
		return reconcile.Result{}, nil
	}
	name := ipa.GetName()
	if mainSpec.IPAddressName != "" && ipa.GetDeletionTimestamp() != nil {
		// The controller does not recreate an IPAddress the owner named, so the owner
		// has to act. A running VM keeps the address while sdn routes it to its pod.
		cb.Message(fmt.Sprintf("The IPAddress %q is being deleted. The virtual machine keeps its address until it stops, but it will not start again until the IPAddress exists: recreate it, or reference another IPAddress in spec.networks[].ipAddressName of the Main network.", name))
		return reconcile.Result{}, nil
	}

	err = network.CheckMainSDNIPAddress(ctx, h.client, vm, ipa)
	var conflict *network.IPAddressConflictError
	switch {
	case errors.Is(err, network.ErrIPAddressOfAnotherNetwork):
		msg := fmt.Sprintf("The IPAddress %q cannot be used for the Main network: it belongs to another network. Specify an IPAddress of the ClusterNetwork %q.", name, network.MainClusterNetworkName)
		cb.Reason(vmcondition.ReasonIPAddressNotAvailable).Message(msg)
		return reconcile.Result{}, nil
	case errors.As(err, &conflict):
		msg := fmt.Sprintf("The IPAddress %q of the Main network cannot be used: the virtual machine %q references it too. Remove the reference from one of them.", name, conflict.VMName)
		cb.Reason(vmcondition.ReasonIPAddressNotAvailable).Message(msg)
		h.recorder.Event(vm, corev1.EventTypeWarning, vmcondition.ReasonIPAddressNotAvailable.String(), msg)
		return reconcile.Result{RequeueAfter: addressConflictRetryInterval}, nil
	case errors.Is(err, network.ErrIPAddressBeingDeleted):
		cb.Message(fmt.Sprintf("The IPAddress %q of the Main network is being deleted: waiting for a new one.", name))
		return reconcile.Result{}, nil
	case errors.Is(err, network.ErrIPAddressNotAllocated):
		switch {
		case network.IPAddressPoolExhausted(ipa):
			cb.Message(fmt.Sprintf("The IPAddress %q cannot be allocated: no free IP addresses left in the pool.", name))
		case network.IPAddressTakenByAnother(ipa):
			cb.Reason(vmcondition.ReasonIPAddressNotAvailable).Message(fmt.Sprintf("The IPAddress %q cannot be allocated: another IPAddress already holds its address. Specify an IPAddress with a free address in spec.networks[].ipAddressName of the Main network.", name))
		default:
			cb.Message(fmt.Sprintf("The IPAddress %q is not allocated yet: waiting for it to be assigned.", name))
		}
		return reconcile.Result{}, nil
	case err != nil:
		cb.Message(fmt.Sprintf("Failed to check the IPAddress of the Main network: %v", err))
		return reconcile.Result{}, err
	}

	cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonIPAddressReady)
	if vm.Status.Phase != v1alpha2.MachineRunning && vm.Status.Phase != v1alpha2.MachineStopping {
		vm.Status.IPAddress = ipa.Status.Address
	}
	// A stale reference would keep the vmip Attached and make a snapshot capture it.
	vm.Status.VirtualMachineIPAddress = ""
	return reconcile.Result{}, h.checkAddressAssignedInGuest(ctx, s, ipa.Status.Address, cb, log)
}

// checkAddressAssignedInGuest sets IPAddressReady to False when a running VM does not
// have its Main address on the default guest interface.
func (h *IPAMHandler) checkAddressAssignedInGuest(ctx context.Context, s state.VirtualMachineState, address string, cb *conditions.ConditionBuilder, log *slog.Logger) error {
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		cb.Reason(vmcondition.ReasonIPAddressNotReady).Message(fmt.Sprintf("Failed to get KVVMI: %v", err))
		return err
	}
	if kvvmi == nil || kvvmi.Status.Phase != virtv1.Running {
		return nil
	}
	i := slices.IndexFunc(kvvmi.Status.Interfaces, func(iface virtv1.VirtualMachineInstanceNetworkInterface) bool {
		return iface.Name == network.NameDefaultInterface
	})
	if i < 0 || slices.Contains(kvvmi.Status.Interfaces[i].IPs, address) {
		return nil
	}
	msg := fmt.Sprintf("IP address (%s) is not among addresses assigned to '%s' network interface (%s)", address, network.NameDefaultInterface, strings.Join(kvvmi.Status.Interfaces[i].IPs, ", "))
	cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonIPAddressNotAssigned).Message(msg)
	log.Warn(msg)
	return nil
}

func (h *IPAMHandler) deleteManagedVMIP(ctx context.Context, s state.VirtualMachineState, vm *v1alpha2.VirtualMachine) error {
	vmip, err := s.IPAddress(ctx)
	if err != nil {
		return err
	}
	if vmip == nil || vm == nil || vmip.Labels[annotations.LabelVirtualMachineUID] != string(vm.GetUID()) {
		return nil
	}
	if err := h.client.Delete(ctx, vmip); err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (h *IPAMHandler) Name() string {
	return nameIpamHandler
}

// createReceiverIPAddress reserves the source address of a received VM. A webhook denial does not always
// mean the address is taken: the previous reconcile may have created the address already, and the cache
// has not seen it yet. The live read tells this cluster's own address from a foreign one.
func (h *IPAMHandler) createReceiverIPAddress(ctx context.Context, vm *v1alpha2.VirtualMachine, receiver *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver, address string) error {
	denial := h.ipam.CreateIPAddress(ctx, vm, h.client, address)
	if !isAdmissionDenial(denial) {
		return denial
	}
	own, err := h.findOwnStaticIPAddress(ctx, vm, address)
	if err != nil {
		return err
	}
	if own != nil {
		return nil
	}
	why := denial.Error()
	if _, after, ok := strings.Cut(why, "denied the request: "); ok {
		why = after
	}
	return peering.SetReceiverFailed(ctx, h.client, receiver, peering.ReceiverReasonIPAddressNotAvailable,
		fmt.Sprintf("the IP address %s is not available in namespace %q: %s. Free the address there and retry the move", address, vm.Namespace, why))
}

func (h *IPAMHandler) findOwnStaticIPAddress(ctx context.Context, vm *v1alpha2.VirtualMachine, address string) (*v1alpha2.VirtualMachineIPAddress, error) {
	reader := h.apiReader
	if reader == nil {
		reader = h.client
	}
	var list v1alpha2.VirtualMachineIPAddressList
	if err := reader.List(ctx, &list, client.InNamespace(vm.Namespace), client.MatchingLabels{annotations.LabelVirtualMachineUID: string(vm.GetUID())}); err != nil {
		return nil, err
	}
	for i := range list.Items {
		if list.Items[i].Spec.StaticIP == address {
			return &list.Items[i], nil
		}
	}
	return nil, nil
}

func isAdmissionDenial(err error) bool {
	return err != nil && (k8serrors.IsInvalid(err) || strings.Contains(err.Error(), "denied the request"))
}
