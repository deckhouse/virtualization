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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmaclease/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaclcondition"
)

const LifecycleHandlerName = "LifecycleHandler"

type LifecycleHandler struct{}

func NewLifecycleHandler() *LifecycleHandler {
	return &LifecycleHandler{}
}

func (h *LifecycleHandler) Handle(ctx context.Context, state state.VMMACLeaseState) (reconcile.Result, error) {
	lease := state.VirtualMachineMACAddressLease()
	leaseStatus := &lease.Status

	if state.ShouldDeletion() {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmmaclcondition.BoundType).
		Generation(lease.GetGeneration()).
		Reason(conditions.ReasonUnknown).
		Status(metav1.ConditionUnknown)

	vmmac, err := state.VirtualMachineMACAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmmac != nil {
		leaseStatus.Phase = virtv2.VirtualMachineMACAddressLeasePhaseBound
		cb.Status(metav1.ConditionTrue).
			Reason(vmmaclcondition.Bound)
		conditions.SetCondition(cb, &leaseStatus.Conditions)
	} else {
		leaseStatus.Phase = virtv2.VirtualMachineMACAddressLeasePhaseReleased
		cb.Status(metav1.ConditionFalse).
			Reason(vmiplcondition.Released).
			Message("VirtualMachineMACAddress lease is not used by any VirtualMachineMACAddress")
		conditions.SetCondition(cb, &leaseStatus.Conditions)
	}

	leaseStatus.ObservedGeneration = lease.GetGeneration()

	return reconcile.Result{}, nil
}

func (h *LifecycleHandler) Name() string {
	return LifecycleHandlerName
}
