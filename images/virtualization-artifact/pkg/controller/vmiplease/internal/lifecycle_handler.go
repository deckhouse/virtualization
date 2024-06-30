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

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

type LifecycleHandler struct {
	client client.Client
	logger logr.Logger
}

func NewLifecycleHandler(client client.Client, logger logr.Logger) *LifecycleHandler {
	return &LifecycleHandler{
		client: client,
		logger: logger.WithValues("handler", "LifecycleHandler"),
	}
}

func (h *LifecycleHandler) Handle(ctx context.Context, state state.VMIPLeaseState) (reconcile.Result, error) {
	changedLease := state.VirtualMachineIPAddressLease().Changed()
	currentLease := state.VirtualMachineIPAddressLease().Current()

	isDeletion := currentLease.DeletionTimestamp != nil

	// Do nothing if object is being deleted as any update will lead to en error.
	if isDeletion {
		return reconcile.Result{}, nil
	}

	mgr := conditions.NewManager(currentLease.Status.Conditions)
	cb := conditions.NewConditionBuilder(vmiplcondition.BoundType).
		Generation(currentLease.GetGeneration())

	vmip, err := state.VirtualMachineIPAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case vmip != nil:
		changedLease.Status.Phase = virtv2.VirtualMachineIPAddressLeasePhaseBound
		mgr.Update(cb.Status(metav1.ConditionTrue).
			Reason(vmiplcondition.Bound).
			Condition())
	case currentLease.Spec.ReclaimPolicy == virtv2.VirtualMachineIPAddressReclaimPolicyRetain:
		changedLease.Status.Phase = virtv2.VirtualMachineIPAddressLeasePhaseReleased
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmiplcondition.Released).
			Condition())
	default:
		// No need to do anything: it should be already in the process of being deleted.
	}
	changedLease.Status.Conditions = mgr.Generate()

	return reconcile.Result{}, nil
}
