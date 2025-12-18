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
	"fmt"
	"log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameIpamHandler = "IPAMHandler"

type IPAM interface {
	IsBound(vmName string, vmip *v1alpha2.VirtualMachineIPAddress) bool
	CheckIPAddressAvailableForBinding(vmName string, vmip *v1alpha2.VirtualMachineIPAddress) error
	CreateIPAddress(ctx context.Context, vm *v1alpha2.VirtualMachine, client client.Client) error
}

func NewIPAMHandler(ipam IPAM, cl client.Client, recorder eventrecord.EventRecorderLogger) *IPAMHandler {
	return &IPAMHandler{
		ipam:     ipam,
		client:   cl,
		recorder: recorder,
	}
}

type IPAMHandler struct {
	ipam     IPAM
	client   client.Client
	recorder eventrecord.EventRecorderLogger
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

	if !hasDefaultNetwork(vm.Status.Networks) {
		vm.Status.IPAddress = ""
		vm.Status.VirtualMachineIPAddress = ""
		if err := h.deleteManagedVMIP(ctx, s); err != nil {
			cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonIPAddressNotReady).
				Message(fmt.Sprintf("Failed to delete VirtualMachineIPAddress: %v", err))
			return reconcile.Result{}, err
		}
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonIPAddressReady)
		return reconcile.Result{}, nil
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
		kvvmi, err := s.KVVMI(ctx)
		if err != nil {
			cb.Reason(vmcondition.ReasonIPAddressNotReady).Message(fmt.Sprintf("Failed to get KVVMI: %v", err))
			return reconcile.Result{}, err
		}
		if kvvmi != nil && kvvmi.Status.Phase == virtv1.Running {
			for _, iface := range kvvmi.Status.Interfaces {
				if iface.Name == network.NameDefaultInterface {
					hasClaimedIP := false
					for _, ip := range iface.IPs {
						if ip == ipAddress.Status.Address {
							hasClaimedIP = true
						}
					}
					if !hasClaimedIP {
						msg := fmt.Sprintf("IP address (%s) is not among addresses assigned to '%s' network interface (%s)", ipAddress.Status.Address, network.NameDefaultInterface, strings.Join(iface.IPs, ", "))
						cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonIPAddressNotAssigned).Message(msg)
						log.Warn(msg)
					}
					break
				}
			}
		}
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionFalse)

	// 2. Ip address not found: create if possible or wait for the ip address.
	if ipAddress == nil {
		cb.Reason(vmcondition.ReasonIPAddressNotReady)
		if vm.Spec.VirtualMachineIPAddress != "" {
			log.Info(fmt.Sprintf("The requested ip address (%s) for the virtual machine not found: waiting for the ip address", vm.Spec.VirtualMachineIPAddress))
			cb.Message(fmt.Sprintf("The requested ip address (%s) for the virtual machine not found: waiting for the ip address", vm.Spec.VirtualMachineIPAddress))
			return reconcile.Result{}, nil
		}
		log.Info("VirtualMachineIPAddress not found: create the new one", slog.String("vmipName", vm.GetName()))
		cb.Message(fmt.Sprintf("VirtualMachineIPAddress %q not found: it may be in the process of being created", vm.GetName()))
		return reconcile.Result{}, h.ipam.CreateIPAddress(ctx, vm, h.client)
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
		msg := fmt.Sprintf("The requested ip address (%s) attached to VirtualMachine '%s': waiting for the ip address", vm.Spec.VirtualMachineIPAddress, ipAddress.Status.VirtualMachine)
		log.Info(msg)
		cb.Reason(vmcondition.ReasonIPAddressNotReady).Message(msg)
		return reconcile.Result{}, nil
	}

	// 5. Ip address exists and available for binding with virtual machine: waiting for the ip address.
	log.Info("Waiting for the ip address to be bound to VM", "vmipName", vm.Spec.VirtualMachineIPAddress)
	cb.Reason(vmcondition.ReasonIPAddressNotReady).Message("Ip address not bound: waiting for the ip address")

	return reconcile.Result{}, nil
}

func hasDefaultNetwork(ns []v1alpha2.NetworksStatus) bool {
	for _, n := range ns {
		if n.Type == v1alpha2.NetworksTypeMain {
			return true
		}
	}
	return false
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
