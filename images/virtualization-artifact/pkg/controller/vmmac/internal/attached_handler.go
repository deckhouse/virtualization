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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

type AttachedHandler struct {
	recorder eventrecord.EventRecorderLogger
	client   client.Client
}

func NewAttachedHandler(recorder eventrecord.EventRecorderLogger, client client.Client) *AttachedHandler {
	return &AttachedHandler{
		recorder: recorder,
		client:   client,
	}
}

func (h *AttachedHandler) Handle(ctx context.Context, vmmac *virtv2.VirtualMachineMACAddress) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmmaccondition.AttachedType).Generation(vmmac.GetGeneration())

	vm, err := h.getAttachedVirtualMachine(ctx, vmmac)
	if err != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message(fmt.Sprintf("Failed to get attached virtual machine: %s.", err))
		conditions.SetCondition(cb, &vmmac.Status.Conditions)
		return reconcile.Result{}, fmt.Errorf("get attached vm: %w", err)
	}

	if vm == nil {
		vmmac.Status.VirtualMachine = ""
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineNotFound).
			Message("VirtualMachineMACAddress is not attached to any virtual machine.")
		conditions.SetCondition(cb, &vmmac.Status.Conditions)
		h.recorder.Event(vmmac, corev1.EventTypeWarning, virtv2.ReasonNotAttached, "VirtualMachineMACAddress is not attached to any virtual machine.")
		return reconcile.Result{}, nil
	}

	vmmac.Status.VirtualMachine = vm.Name
	cb.
		Status(metav1.ConditionTrue).
		Reason(vmmaccondition.Attached).
		Message("")
	conditions.SetCondition(cb, &vmmac.Status.Conditions)
	h.recorder.Eventf(vmmac, corev1.EventTypeNormal, virtv2.ReasonAttached, "VirtualMachineMACAddress is attached to \"%s/%s\".", vm.Namespace, vm.Name)

	return reconcile.Result{}, nil
}

func (h *AttachedHandler) getAttachedVirtualMachine(ctx context.Context, vmmac *virtv2.VirtualMachineMACAddress) (*virtv2.VirtualMachine, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{Namespace: vmmac.Namespace})
	if err != nil {
		return nil, fmt.Errorf("list vms: %w", err)
	}

	// Return the first one for which the status matches.
	// If no status matches, return the first one for which the spec matches.
	var found bool
	var attachedVM *virtv2.VirtualMachine
	for _, vm := range vms.Items {
		for _, ns := range vm.Status.Networks {
			if ns.VirtualMachineMACAddressName == vmmac.Name {
				attachedVM = &vm
				found = true
				break
			}
		}

		if found {
			break
		}
	}

	if attachedVM == nil {
		for _, vm := range vms.Items {
			if attachedVM == nil {
				for _, ns := range vm.Spec.Networks {
					if ns.VirtualMachineMACAddressName == vmmac.Name {
						attachedVM = &vm
					}
				}
			}

			if found {
				break
			}
		}
	}

	return attachedVM, nil
}
