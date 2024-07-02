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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
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

func (h *LifecycleHandler) Handle(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	// Do nothing if object is being deleted as any update will lead to en error.
	isDeletion := state.VirtualMachineIP().Current().DeletionTimestamp != nil
	if isDeletion {
		return reconcile.Result{}, nil
	}

	vmipStatus := state.VirtualMachineIP().Current().Status.DeepCopy()
	state.VirtualMachineIP()
	vmipStatus.VirtualMachine = ""

	vm, err := state.VirtualMachine(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vm != nil {
		vmipStatus.VirtualMachine = vm.Name
	}

	vmipStatus.Address = ""
	vmipStatus.ConflictMessage = ""
	mgr := conditions.NewManager(vmipStatus.Conditions)
	cb := conditions.NewConditionBuilder(vmipcondition.Bound).
		Generation(state.VirtualMachineIP().Current().GetGeneration())

	vmipLease, err := state.VirtualMachineIPLease(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case vmipLease == nil && state.VirtualMachineIP().Current().Spec.VirtualMachineIPAddressLease != "":
		vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhaseLost
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseLost).
			Condition())

	case vmipLease == nil:
		vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhasePending
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotFound).
			Condition())

	case util.IsBoundLease(vmipLease, state.VirtualMachineIP()):
		vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhaseBound
		vmipStatus.Address = state.VirtualMachineIP().Current().Spec.Address
		mgr.Update(cb.Status(metav1.ConditionTrue).
			Reason(vmipcondition.Bound).
			Condition())

		mgr.Update(conditions.NewConditionBuilder(vmipcondition.Attached).
			Generation(state.VirtualMachineIP().Current().GetGeneration()).Status(metav1.ConditionTrue).
			Reason(vmipcondition.Attached).
			Condition())

	case vmipLease.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseBound:
		vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhaseConflict

		// There is only one way to automatically link Ip Address in phase Conflict with recently released Lease: only with cyclic reconciliation (with an interval of N seconds).
		// At the moment this looks redundant, so Ip Address in the phase Conflict will not be able to bind the recently released Lease.
		// It is necessary to recreate Ip Address manually in order to link it to released Lease.
		vmipStatus.ConflictMessage = "Lease is bounded to another VirtualMachineIP: please recreate VMIP when the lease is released"
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseAlready).
			Condition())

	default:
		vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhasePending
		mgr.Update(cb.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotFound).
			Condition())
	}

	h.logger.Info("Set VirtualMachineIP phase", "phase", vmipStatus.Phase)
	vmipStatus.Conditions = mgr.Generate()
	state.VirtualMachineIP().Changed().Status = *vmipStatus

	return reconcile.Result{}, nil
}
