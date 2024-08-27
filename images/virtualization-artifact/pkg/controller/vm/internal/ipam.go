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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameIpamHandler = "IPAMHandler"

type IPAM interface {
	IsBound(vmName string, vmip *virtv2.VirtualMachineIPAddress) bool
	CheckIpAddressAvailableForBinding(vmName string, vmip *virtv2.VirtualMachineIPAddress) error
	CreateIPAddress(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client) error
}

func NewIPAMHandler(ipam IPAM, cl client.Client, recorder record.EventRecorder) *IPAMHandler {
	return &IPAMHandler{
		ipam:       ipam,
		client:     cl,
		recorder:   recorder,
		protection: service.NewProtectionService(cl, virtv2.FinalizerIPAddressProtection),
	}
}

type IPAMHandler struct {
	ipam       IPAM
	client     client.Client
	recorder   record.EventRecorder
	protection *service.ProtectionService
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

	//nolint:staticcheck
	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder(vmcondition.TypeIPAddressReady).
		Generation(current.GetGeneration())

	ipAddress, err := s.IPAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isDeletion(current) {
		return reconcile.Result{}, h.protection.RemoveProtection(ctx, ipAddress)
	}
	err = h.protection.AddProtection(ctx, ipAddress)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 1. OK: already bound.
	if h.ipam.IsBound(current.GetName(), ipAddress) {
		mgr.Update(cb.Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonIPAddressReady).
			Condition())
		changed.Status.VirtualMachineIPAddress = ipAddress.GetName()
		changed.Status.IPAddress = ipAddress.Status.Address
		kvvmi, err := s.KVVMI(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}
		if kvvmi != nil && kvvmi.Status.Phase == virtv1.Running {
			for _, iface := range kvvmi.Status.Interfaces {
				if iface.Name == kvbuilder.NetworkInterfaceName {
					hasClaimedIP := false
					for _, ip := range iface.IPs {
						if ip == ipAddress.Status.Address {
							hasClaimedIP = true
						}
					}
					if !hasClaimedIP {
						msg := fmt.Sprintf("IP address (%s) is not among addresses assigned to '%s' network interface (%s)", ipAddress.Status.Address, kvbuilder.NetworkInterfaceName, strings.Join(iface.IPs, ", "))
						mgr.Update(cb.Status(metav1.ConditionFalse).
							Reason(vmcondition.ReasonIPAddressNotAssigned).
							Message(msg).
							Condition())
						h.recorder.Event(changed, corev1.EventTypeWarning, vmcondition.ReasonIPAddressNotAssigned.String(), msg)
						log.Error(msg)
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
			return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
		}
		log.Info("VirtualMachineIPAddress not found: create the new one", slog.String("vmipName", current.GetName()))
		cb.Message(fmt.Sprintf("VirtualMachineIPAddress %q not found: it may be in the process of being created", current.GetName()))
		mgr.Update(cb.Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{RequeueAfter: 2 * time.Second}, h.ipam.CreateIPAddress(ctx, changed, h.client)
	}

	// 3. Check if possible to bind virtual machine with the found ip address.
	err = h.ipam.CheckIpAddressAvailableForBinding(current.GetName(), ipAddress)
	if err != nil {
		log.Info("Ip address is not available to be bound", "err", err, "vmipName", current.Spec.VirtualMachineIPAddress)
		reason := vmcondition.ReasonIPAddressNotAvailable.String()
		//nolint:staticcheck
		mgr.Update(cb.Reason(conditions.DeprecatedWrappedString(reason)).Message(err.Error()).Condition())
		changed.Status.Conditions = mgr.Generate()
		h.recorder.Event(changed, corev1.EventTypeWarning, reason, err.Error())
		return reconcile.Result{}, nil
	}

	// 4. Ip address exists and available for binding with virtual machine: waiting for the ip address.
	log.Info("Waiting for the ip address to be bound to VM", "vmipName", current.Spec.VirtualMachineIPAddress)
	mgr.Update(cb.Reason(vmcondition.ReasonIPAddressNotReady).
		Message("Ip address not bound: waiting for the ip address").Condition())
	changed.Status.Conditions = mgr.Generate()

	return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
}

func (h *IPAMHandler) Name() string {
	return nameIpamHandler
}
