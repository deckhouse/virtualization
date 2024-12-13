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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameMACAMHandler = "MACAMHandler"

type MACAM interface {
	IsBound(vmName string, vmmac *virtv2.VirtualMachineMACAddress) bool
	CheckMACAddressAvailableForBinding(vmmac *virtv2.VirtualMachineMACAddress) error
	CreateMACAddress(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client) error
}

func NewMACAMHandler(macam MACAM, cl client.Client, recorder eventrecord.EventRecorderLogger) *MACAMHandler {
	return &MACAMHandler{
		macam:      macam,
		client:     cl,
		recorder:   recorder,
		protection: service.NewProtectionService(cl, virtv2.FinalizerMACAddressProtection),
	}
}

type MACAMHandler struct {
	macam      MACAM
	client     client.Client
	recorder   eventrecord.EventRecorderLogger
	protection *service.ProtectionService
}

func (h *MACAMHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(nameMACAMHandler))

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, vmcondition.TypeMACAddressReady); update {
		return reconcile.Result{Requeue: true}, nil
	}

	//nolint:staticcheck
	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder(vmcondition.TypeMACAddressReady).
		Generation(current.GetGeneration())

	macAddress, err := s.MACAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isDeletion(current) {
		return reconcile.Result{}, h.protection.RemoveProtection(ctx, macAddress)
	}
	err = h.protection.AddProtection(ctx, macAddress)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 1. OK: already bound.
	if h.macam.IsBound(current.GetName(), macAddress) {
		mgr.Update(cb.Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonMACAddressReady).
			Condition())
		changed.Status.VirtualMachineMACAddress = macAddress.GetName()
		if changed.Status.Phase != virtv2.MachineRunning && changed.Status.Phase != virtv2.MachineStopping {
			changed.Status.MACAddress = macAddress.Status.Address
		}

		// todo dlopatin add check mac address on kvvmi and update condition status

		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionFalse)

	// 2. MAC address not found: create if possible or wait for the MAC address.
	if macAddress == nil {
		cb.Reason(vmcondition.ReasonMACAddressNotReady)
		if current.Spec.VirtualMachineMACAddress != "" {
			log.Info(fmt.Sprintf("The requested MAC address (%s) for the virtual machine not found: waiting for the MAC address", current.Spec.VirtualMachineMACAddress))
			cb.Message(fmt.Sprintf("The requested MAC address (%s) for the virtual machine not found: waiting for the MAC address", current.Spec.VirtualMachineMACAddress))
			mgr.Update(cb.Condition())
			changed.Status.Conditions = mgr.Generate()
			return reconcile.Result{}, nil
		}
		log.Info("VirtualMachineMACAddress not found: create the new one", slog.String("vmmacName", current.GetName()))
		cb.Message(fmt.Sprintf("VirtualMachineMACAddress %q not found: it may be in the process of being created", current.GetName()))
		mgr.Update(cb.Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, h.macam.CreateMACAddress(ctx, changed, h.client)
	}

	// 3. Check if possible to bind virtual machine with the found MAC address.
	err = h.macam.CheckMACAddressAvailableForBinding(macAddress)
	if err != nil {
		log.Info("MAC address is not available to be bound", "err", err, "vmmacName", current.Spec.VirtualMachineMACAddress)
		reason := vmcondition.ReasonMACAddressNotAvailable
		mgr.Update(cb.Reason(reason).Message(err.Error()).Condition())
		changed.Status.Conditions = mgr.Generate()
		h.recorder.Event(changed, corev1.EventTypeWarning, reason.String(), err.Error())
		return reconcile.Result{}, nil
	}

	// 4. MAC address exist and attached to another VirtualMachine
	if macAddress.Status.VirtualMachine != "" && macAddress.Status.VirtualMachine != changed.Name {
		msg := fmt.Sprintf("The requested MAC address (%s) attached to VirtualMachine '%s': waiting for the MAC address", current.Spec.VirtualMachineMACAddress, macAddress.Status.VirtualMachine)
		log.Info(msg)
		mgr.Update(cb.Reason(vmcondition.ReasonMACAddressNotReady).
			Message(msg).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	// 5. MAC address exists and available for binding with virtual machine: waiting for the MAC address.
	log.Info("Waiting for the MAC address to be bound to VM", "vmmacName", current.Spec.VirtualMachineMACAddress)
	mgr.Update(cb.Reason(vmcondition.ReasonMACAddressNotReady).
		Message("MAC address not bound: waiting for the MAC address").Condition())
	changed.Status.Conditions = mgr.Generate()

	return reconcile.Result{}, nil
}

func (h *MACAMHandler) Name() string {
	return nameMACAMHandler
}
