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

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

const LifecycleHandlerName = "LifecycleHandler"

type LifecycleHandler struct {
	logger logr.Logger
}

func NewLifecycleHandler(logger logr.Logger) *LifecycleHandler {
	return &LifecycleHandler{
		logger: logger.WithValues("handler", LifecycleHandlerName),
	}
}

func (h *LifecycleHandler) Handle(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	vmip := state.VirtualMachineIP()
	vmipStatus := &vmip.Status

	vm, err := state.VirtualMachine(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	mgr := conditions.NewManager(vmipStatus.Conditions)
	conditionBound := conditions.NewConditionBuilder(vmipcondition.BoundType).
		Generation(vmip.GetGeneration())

	conditionAttach := conditions.NewConditionBuilder(vmipcondition.AttachedType).
		Generation(vmip.GetGeneration())

	if vm != nil {
		vmipStatus.VirtualMachine = vm.Name
		mgr.Update(conditionAttach.Status(metav1.ConditionTrue).
			Reason(vmipcondition.Attached).
			Condition())
	} else {
		vmipStatus.VirtualMachine = ""
		mgr.Update(conditionAttach.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineNotFound).
			Message("Virtual machine not found").
			Condition())
	}

	lease, err := state.VirtualMachineIPLease(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case lease == nil && vmipStatus.Address != "":
		if vmipStatus.Phase != virtv2.VirtualMachineIPAddressPhasePending {
			vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhasePending
			mgr.Update(conditionBound.Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressLeaseLost).
				Message(fmt.Sprintf("VirtualMachineIPAddressLease %s doesn't exist",
					common.IpToLeaseName(vmipStatus.Address))).
				Condition())
		}

	case lease == nil:
		if vmipStatus.Phase != virtv2.VirtualMachineIPAddressPhasePending {
			vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhasePending
			mgr.Update(conditionBound.Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressLeaseNotFound).
				Message("VirtualMachineIPAddressLease is not found").
				Condition())
		}

	case util.IsBoundLease(lease, vmip):
		if vmipStatus.Phase != virtv2.VirtualMachineIPAddressPhaseBound {
			vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhaseBound
			vmipStatus.Address = common.LeaseNameToIP(lease.Name)
			mgr.Update(conditionBound.Status(metav1.ConditionTrue).
				Reason(vmipcondition.Bound).
				Condition())
		}

	case lease.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseBound:
		if vmipStatus.Phase != virtv2.VirtualMachineIPAddressPhasePending {
			vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhasePending
			mgr.Update(conditionBound.Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressLeaseAlready).
				Message(fmt.Sprintf("VirtualMachineIPAddressLease %s is bound to another VirtualMachineIP",
					common.IpToLeaseName(vmipStatus.Address))).
				Condition())
		}

	default:
		if vmipStatus.Phase != virtv2.VirtualMachineIPAddressPhasePending {
			vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhasePending
			mgr.Update(conditionBound.Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
				Message(fmt.Sprintf("VirtualMachineIPAddressLease %s is not ready",
					common.IpToLeaseName(vmipStatus.Address))).
				Condition())
		}
	}

	h.logger.Info("Set VirtualMachineIP phase", "phase", vmipStatus.Phase)
	vmipStatus.Conditions = mgr.Generate()
	vmipStatus.ObservedGeneration = vmip.GetGeneration()

	return reconcile.Result{}, nil
}

func (h *LifecycleHandler) Name() string {
	return LifecycleHandlerName
}
