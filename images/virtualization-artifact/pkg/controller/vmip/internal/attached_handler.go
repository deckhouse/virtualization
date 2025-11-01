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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
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

func (h *AttachedHandler) Handle(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmipcondition.AttachedType).Generation(vmip.GetGeneration())

	vm, err := h.getAttachedVirtualMachine(ctx, vmip)
	if err != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message(fmt.Sprintf("Failed to get attached virtual machine: %s.", err))
		conditions.SetCondition(cb, &vmip.Status.Conditions)
		return reconcile.Result{}, fmt.Errorf("get attached vm: %w", err)
	}

	if vm == nil {
		vmip.Status.VirtualMachine = ""
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineNotFound).
			Message("VirtualMachineIPAddress is not attached to any virtual machine.")
		conditions.SetCondition(cb, &vmip.Status.Conditions)
		h.recorder.Event(vmip, corev1.EventTypeWarning, v1alpha2.ReasonNotAttached, "VirtualMachineIPAddress is not attached to any virtual machine.")
		return reconcile.Result{}, nil
	}

	vmip.Status.VirtualMachine = vm.Name
	cb.
		Status(metav1.ConditionTrue).
		Reason(vmipcondition.Attached).
		Message("")
	conditions.SetCondition(cb, &vmip.Status.Conditions)
	h.recorder.Eventf(vmip, corev1.EventTypeNormal, v1alpha2.ReasonAttached, "VirtualMachineIPAddress is attached to \"%s/%s\".", vm.Namespace, vm.Name)

	return reconcile.Result{}, nil
}

func (h *AttachedHandler) getAttachedVirtualMachine(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (*v1alpha2.VirtualMachine, error) {
	var vms v1alpha2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.MatchingFields{
		indexer.IndexFieldVMByIP: vmip.Status.Address,
	})
	if err != nil {
		return nil, fmt.Errorf("list vms: %w", err)
	}

	// Return the first one for which the status matches.
	// If no status matches, return the first one for which the spec matches.
	var attachedVM *v1alpha2.VirtualMachine
	for _, vm := range vms.Items {
		if vm.Status.VirtualMachineIPAddress == vmip.Name {
			attachedVM = &vm
			break
		}

		if attachedVM == nil && vm.Spec.VirtualMachineIPAddress == vmip.Name {
			attachedVM = &vm
		}
	}

	if attachedVM != nil {
		return attachedVM, nil
	}

	// If there's no match for the spec either, then try to find the vm by ownerRef.
	var vmName string
	for _, ownerRef := range vmip.OwnerReferences {
		if ownerRef.Kind == v1alpha2.VirtualMachineKind && string(ownerRef.UID) == vmip.Labels[annotations.LabelVirtualMachineUID] {
			vmName = ownerRef.Name
			break
		}
	}

	if vmName == "" {
		return nil, nil
	}

	vmKey := types.NamespacedName{Name: vmName, Namespace: vmip.Namespace}
	attachedVM, err = object.FetchObject(ctx, vmKey, h.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("fetch vm %s: %w", vmKey, err)
	}

	return attachedVM, nil
}
