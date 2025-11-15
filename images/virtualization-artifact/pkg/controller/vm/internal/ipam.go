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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, vmcondition.TypeIPAddressReady); update {
		return reconcile.Result{Requeue: true}, nil
	}

	//nolint:staticcheck // it's deprecated.
	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder(vmcondition.TypeIPAddressReady).
		Generation(current.GetGeneration())

	ipAddress, err := s.IPAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	// 1. OK: already bound.
	if h.ipam.IsBound(current.GetName(), ipAddress) {
		mgr.Update(cb.Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonIPAddressReady).
			Condition())
		changed.Status.VirtualMachineIPAddress = ipAddress.GetName()
		if changed.Status.Phase != v1alpha2.MachineRunning && changed.Status.Phase != v1alpha2.MachineStopping {
			changed.Status.IPAddress = ipAddress.Status.Address
		}
		kvvmi, err := s.KVVMI(ctx)
		if err != nil {
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
						mgr.Update(cb.Status(metav1.ConditionFalse).
							Reason(vmcondition.ReasonIPAddressNotAssigned).
							Message(msg).
							Condition())
						log.Warn(msg)
					}
					break
				}
			}
		}
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionFalse)

	// 2. Ip address not found: create if possible or wait for the ip address.
	if ipAddress == nil {
		cb.Reason(vmcondition.ReasonIPAddressNotReady)
		if current.Spec.VirtualMachineIPAddress != "" {
			log.Info(fmt.Sprintf("The requested ip address (%s) for the virtual machine not found: waiting for the ip address", current.Spec.VirtualMachineIPAddress))
			cb.Message(fmt.Sprintf("The requested ip address (%s) for the virtual machine not found: waiting for the ip address", current.Spec.VirtualMachineIPAddress))
			mgr.Update(cb.Condition())
			changed.Status.Conditions = mgr.Generate()
			return reconcile.Result{}, nil
		}
		log.Info("VirtualMachineIPAddress not found: create the new one", slog.String("vmipName", current.GetName()))
		cb.Message(fmt.Sprintf("VirtualMachineIPAddress %q not found: it may be in the process of being created", current.GetName()))
		mgr.Update(cb.Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, h.ipam.CreateIPAddress(ctx, changed, h.client)
	}

	// 3. Check if possible to bind virtual machine with the found ip address.
	err = h.ipam.CheckIPAddressAvailableForBinding(current.GetName(), ipAddress)
	if err != nil {
		log.Info("Ip address is not available to be bound", "err", err, "vmipName", current.Spec.VirtualMachineIPAddress)
		reason := vmcondition.ReasonIPAddressNotAvailable
		mgr.Update(cb.Reason(reason).Message(err.Error()).Condition())
		changed.Status.Conditions = mgr.Generate()
		h.recorder.Event(changed, corev1.EventTypeWarning, reason.String(), err.Error())
		return reconcile.Result{}, nil
	}

	// 4. Ip address exist and attached to another VirtualMachine
	if ipAddress.Status.VirtualMachine != "" && ipAddress.Status.VirtualMachine != changed.Name {
		msg := fmt.Sprintf("The requested ip address (%s) attached to VirtualMachine '%s': waiting for the ip address", current.Spec.VirtualMachineIPAddress, ipAddress.Status.VirtualMachine)
		log.Info(msg)
		mgr.Update(cb.Reason(vmcondition.ReasonIPAddressNotReady).
			Message(msg).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	// 5. Ip address exists and available for binding with virtual machine: waiting for the ip address.
	log.Info("Waiting for the ip address to be bound to VM", "vmipName", current.Spec.VirtualMachineIPAddress)
	mgr.Update(cb.Reason(vmcondition.ReasonIPAddressNotReady).
		Message("Ip address not bound: waiting for the ip address").Condition())
	changed.Status.Conditions = mgr.Generate()

	return reconcile.Result{}, nil
}

func (h *IPAMHandler) Name() string {
	return nameIpamHandler
}
