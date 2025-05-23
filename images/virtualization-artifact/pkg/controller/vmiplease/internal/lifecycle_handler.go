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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

type LifecycleHandler struct {
	client client.Client
}

func NewLifecycleHandler(client client.Client) *LifecycleHandler {
	return &LifecycleHandler{
		client: client,
	}
}

func (h *LifecycleHandler) Handle(ctx context.Context, lease *virtv2.VirtualMachineIPAddressLease) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmiplcondition.BoundType).Generation(lease.GetGeneration())

	vmipKey := types.NamespacedName{Name: lease.Spec.VirtualMachineIPAddressRef.Name, Namespace: lease.Spec.VirtualMachineIPAddressRef.Namespace}
	vmip, err := object.FetchObject(ctx, vmipKey, h.client, &virtv2.VirtualMachineIPAddress{})
	if err != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message(fmt.Sprintf("Failed to fetch VirtualMachineIPAddress: %s.", err))
		conditions.SetCondition(cb, &lease.Status.Conditions)
		return reconcile.Result{}, fmt.Errorf("fetch vmip %s: %w", vmipKey, err)
	}

	// Lease is Bound, if there is a vmip with matched Ref.
	if isBound(lease, vmip) {
		lease.Status.Phase = virtv2.VirtualMachineIPAddressLeasePhaseBound
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmiplcondition.Bound).
			Message("")
		conditions.SetCondition(cb, &lease.Status.Conditions)
	} else {
		annotations.AddLabel(lease, annotations.LabelVirtualMachineIPAddressUID, "")
		if lease.Spec.VirtualMachineIPAddressRef != nil {
			lease.Spec.VirtualMachineIPAddressRef.Name = ""
		}

		lease.Status.Phase = virtv2.VirtualMachineIPAddressLeasePhaseReleased
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmiplcondition.Released).
			Message("VirtualMachineIPAddressLease is not bound to any VirtualMachineIPAddress.")
		conditions.SetCondition(cb, &lease.Status.Conditions)
	}

	return reconcile.Result{}, nil
}

func isBound(lease *virtv2.VirtualMachineIPAddressLease, vmip *virtv2.VirtualMachineIPAddress) bool {
	if lease == nil || vmip == nil {
		return false
	}

	if lease.Spec.VirtualMachineIPAddressRef == nil {
		return false
	}

	vmipRef := lease.Spec.VirtualMachineIPAddressRef
	if vmip.Name != vmipRef.Name || vmip.Namespace != vmipRef.Namespace {
		return false
	}

	if string(vmip.UID) != lease.Labels[annotations.LabelVirtualMachineIPAddressUID] {
		return false
	}

	if vmip.Status.Address != "" && vmip.Status.Address != ip.LeaseNameToIP(lease.Name) {
		return false
	}

	return true
}
