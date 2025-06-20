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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

type LifecycleHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		client:   client,
		recorder: recorder,
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
		annotations.AddLabel(lease, annotations.LabelVirtualMachineIPAddressUID, string(vmip.UID))
		if lease.Status.Phase != virtv2.VirtualMachineIPAddressLeasePhaseBound {
			h.recorder.Eventf(lease, corev1.EventTypeNormal, virtv2.ReasonBound, "VirtualMachineIPAddressLease is bound to \"%s/%s\".", vmip.Namespace, vmip.Name)
		}
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

		if lease.Status.Phase != virtv2.VirtualMachineIPAddressLeasePhaseReleased {
			h.recorder.Eventf(lease, corev1.EventTypeWarning, virtv2.ReasonReleased, "VirtualMachineIPAddressLease is released.")
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

	vmipRef := lease.Spec.VirtualMachineIPAddressRef
	if vmipRef == nil || vmip.Name != vmipRef.Name || vmip.Namespace != vmipRef.Namespace {
		return false
	}

	if vmip.Status.Address != "" && vmip.Status.Address != ip.LeaseNameToIP(lease.Name) {
		return false
	}

	return true
}
