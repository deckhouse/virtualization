/*
Copyright 2025 Flant JSC

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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameMACHandler = "MACHandler"

type MACManager interface {
	IsBound(vmName string, vmmac *v1alpha2.VirtualMachineMACAddress) bool
	CheckMACAddressAvailableForBinding(vmmac *v1alpha2.VirtualMachineMACAddress) error
	CreateMACAddress(ctx context.Context, vm *v1alpha2.VirtualMachine, client client.Client, macAddress string) error
}

func NewMACHandler(mac MACManager, cl client.Client, recorder eventrecord.EventRecorderLogger) *MACHandler {
	return &MACHandler{
		macManager: mac,
		client:     cl,
		recorder:   recorder,
	}
}

type MACHandler struct {
	macManager MACManager
	client     client.Client
	recorder   eventrecord.EventRecorderLogger
}

func (h *MACHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	vm := s.VirtualMachine().Changed()

	if isDeletion(vm) {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeMACAddressReady).
		Status(metav1.ConditionUnknown).
		Reason(conditions.ReasonUnknown).
		Generation(vm.GetGeneration())

	defer func() {
		conditions.SetCondition(cb, &vm.Status.Conditions)
	}()

	if vm.Spec.Networks == nil {
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMACAddressReady).Message("")
		return reconcile.Result{}, nil
	}

	vmmacs, err := s.VirtualMachineMACAddresses(ctx)
	if err != nil {
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonMACAddressNotReady).Message(fmt.Sprintf("Failed to get VirtualMachineMACAddresses: %s", err))
		return reconcile.Result{}, err
	}

	expectedMACAddresses := 0
	for _, network := range vm.Spec.Networks {
		// 'Main' network not require a MAC address.
		if network.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		expectedMACAddresses++
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonMACAddressNotReady).Message(fmt.Sprintf("Failed to get KVVM: %s", err))
		return reconcile.Result{}, err
	}
	if len(vmmacs) < expectedMACAddresses {
		createdCount := 0
		if kvvm != nil && len(vmmacs) == 0 {
			for _, iface := range kvvm.Spec.Template.Spec.Domain.Devices.Interfaces {
				if strings.HasPrefix(iface.Name, "veth_") {
					err = h.macManager.CreateMACAddress(ctx, vm, h.client, iface.MacAddress)
					createdCount++
					if err != nil {
						cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonMACAddressNotReady).Message(fmt.Sprintf("Failed to create VirtualMachineMACAddress: %s", err))
						return reconcile.Result{}, err
					}
				}
			}
		}
		if createdCount < expectedMACAddresses {
			macsToCreate := countNetworksWithMACRequest(vm.Spec.Networks, vmmacs) - createdCount
			for i := 0; i < macsToCreate; i++ {
				err = h.macManager.CreateMACAddress(ctx, vm, h.client, "")
				if err != nil {
					cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonMACAddressNotReady).Message(fmt.Sprintf("Failed to create VirtualMachineMACAddress: %s", err))
					return reconcile.Result{}, err
				}
			}
		}

		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonMACAddressNotReady).Message(fmt.Sprintf("Waiting for the MAC address to be created %d/%d", len(vmmacs), expectedMACAddresses))
		return reconcile.Result{}, nil
	}

	var notReadyMessages []string
	allReady := true

	log := logger.FromContext(ctx).With(logger.SlogHandler(nameMACHandler))
	for _, macAddress := range vmmacs {
		// 1. OK: already bound.
		if h.macManager.IsBound(vm.GetName(), macAddress) {
			continue
		}

		allReady = false
		// 2. Check if possible to bind virtual machine with the found MAC address.
		err = h.macManager.CheckMACAddressAvailableForBinding(macAddress)
		if err != nil {
			msg := fmt.Sprintf("VirtualMachineMACAddress %s: is not available to be bound (%v)", macAddress.Name, err)
			log.Info(msg)
			notReadyMessages = append(notReadyMessages, msg)
			h.recorder.Event(vm, corev1.EventTypeWarning, vmcondition.ReasonMACAddressNotAvailable.String(), err.Error())
			continue
		}

		// 3. VirtualMachineMACAddress exist and attached to another VirtualMachine
		if macAddress.Status.VirtualMachine != "" && macAddress.Status.VirtualMachine != vm.Name {
			msg := fmt.Sprintf("The requested VirtualMachineMACAddress (%s) attached to VirtualMachine '%s': please remove VirtualMachine '%s' or reconfigure it", macAddress.Name, macAddress.Status.VirtualMachine, macAddress.Status.VirtualMachine)
			log.Info(msg)
			notReadyMessages = append(notReadyMessages, msg)
			continue
		}

		// 4. VirtualMachineMACAddress exists and available for binding with virtual machine: waiting for the MAC address.
		msg := fmt.Sprintf("VirtualMachineMACAddress %s: waiting for MAC address binding", macAddress.Name)
		notReadyMessages = append(notReadyMessages, msg)
		log.Info(msg)
	}

	if allReady {
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonMACAddressReady).Message("")
	} else {
		finalMessage := strings.Join(notReadyMessages, "; ")
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonMACAddressNotReady).Message(finalMessage)
	}

	return reconcile.Result{}, nil
}

func (h *MACHandler) Name() string {
	return nameMACHandler
}

func countNetworksWithMACRequest(networkSpec []v1alpha2.NetworksSpec, vmmacs []*v1alpha2.VirtualMachineMACAddress) int {
	existingMACNames := make(map[string]bool)
	for _, vmmac := range vmmacs {
		existingMACNames[vmmac.Name] = true
	}

	count := 0
	for _, ns := range networkSpec {
		if ns.Type == v1alpha2.NetworksTypeMain {
			continue
		}

		if ns.VirtualMachineMACAddressName == "" {
			count++
		} else if !existingMACNames[ns.VirtualMachineMACAddressName] {
			count++
		}
	}

	return count
}
