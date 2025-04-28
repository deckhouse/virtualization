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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

const LifecycleHandlerName = "LifecycleHandler"

type LifecycleHandler struct{}

func NewLifecycleHandler() *LifecycleHandler {
	return &LifecycleHandler{}
}

func (h *LifecycleHandler) Handle(ctx context.Context, state state.VMIPLeaseState) (reconcile.Result, error) {
	lease := state.VirtualMachineIPAddressLease()
	leaseStatus := &lease.Status

	// Do nothing if object is being deleted as any update will lead to en error.
	if state.ShouldDeletion() {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmiplcondition.BoundType).
		Generation(lease.GetGeneration()).
		Reason(conditions.ReasonUnknown).
		Status(metav1.ConditionUnknown)

	vmip, err := state.VirtualMachineIPAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmip != nil && vmip.Status.Address == ip.LeaseNameToIP(lease.Name) {
		if leaseStatus.Phase != virtv2.VirtualMachineIPAddressLeasePhaseBound {
			leaseStatus.Phase = virtv2.VirtualMachineIPAddressLeasePhaseBound
			cb.Status(metav1.ConditionTrue).
				Reason(vmiplcondition.Bound)
			conditions.SetCondition(cb, &leaseStatus.Conditions)
		}
	} else {
		if leaseStatus.Phase != virtv2.VirtualMachineIPAddressLeasePhaseReleased {
			leaseStatus.Phase = virtv2.VirtualMachineIPAddressLeasePhaseReleased
			cb.Status(metav1.ConditionFalse).
				Reason(vmiplcondition.Released).
				Message("VirtualMachineIPAddress lease is not used by any VirtualMachineIPAddress")
			conditions.SetCondition(cb, &leaseStatus.Conditions)
		}
	}

	leaseStatus.ObservedGeneration = lease.GetGeneration()

	return reconcile.Result{}, nil
}

func (h *LifecycleHandler) Name() string {
	return LifecycleHandlerName
}
