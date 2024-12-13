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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/util"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

const LifecycleHandlerName = "LifecycleHandler"

type LifecycleHandler struct{}

func NewLifecycleHandler() *LifecycleHandler {
	return &LifecycleHandler{}
}

func (h LifecycleHandler) Handle(ctx context.Context, state state.VMMACState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(LifecycleHandlerName))

	mac := state.VirtualMachineMAC()
	macStatus := &mac.Status

	vm, err := state.VirtualMachine(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	conditionBound := conditions.NewConditionBuilder(vmmaccondition.BoundType).
		Generation(mac.GetGeneration()).
		Reason(conditions.ReasonUnknown).
		Status(metav1.ConditionUnknown)

	conditionAttach := conditions.NewConditionBuilder(vmmaccondition.AttachedType).
		Generation(mac.GetGeneration()).
		Reason(conditions.ReasonUnknown).
		Status(metav1.ConditionUnknown)

	defer func() {
		conditions.SetCondition(conditionBound, &macStatus.Conditions)
		conditions.SetCondition(conditionAttach, &macStatus.Conditions)
	}()

	if vm == nil || vm.DeletionTimestamp != nil {
		macStatus.VirtualMachine = ""
		conditionAttach.Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineNotFound).
			Message("Virtual machine not found")
	}

	lease, err := state.VirtualMachineMACLease(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	needRequeue := false
	switch {
	case lease == nil && macStatus.Address != "":
		macStatus.Phase = virtv2.VirtualMachineMACAddressPhasePending
		conditionBound.Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseLost).
			Message(fmt.Sprintf("VirtualMachineMACAddressLease %s doesn't exist",
				ip.IpToLeaseName(macStatus.Address)))

	case lease == nil:
		macStatus.Phase = virtv2.VirtualMachineMACAddressPhasePending
		conditionBound.Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotFound).
			Message("VirtualMachineMACAddressLease is not found")

	case vm != nil && vm.GetDeletionTimestamp().IsZero():
		macStatus.Phase = virtv2.VirtualMachineMACAddressPhaseAttached
		macStatus.VirtualMachine = vm.Name
		conditionAttach.Status(metav1.ConditionTrue).
			Reason(vmmaccondition.Attached)

	case util.IsBoundLease(lease, mac):
		macStatus.Phase = virtv2.VirtualMachineMACAddressPhaseBound
		macStatus.Address = ip.LeaseNameToIP(lease.Name)
		conditionBound.Status(metav1.ConditionTrue).
			Reason(vmmaccondition.Bound)

	case lease.Status.Phase == virtv2.VirtualMachineMACAddressLeasePhaseBound:
		macStatus.Phase = virtv2.VirtualMachineMACAddressPhasePending
		log.Warn(fmt.Sprintf("VirtualMachineMACAddressLease %s is bound to another VirtualMachineMACAddress resource: %s/%s",
			lease.Name, lease.Spec.VirtualMachineMACAddressRef.Name, lease.Spec.VirtualMachineMACAddressRef.Namespace))
		conditionBound.Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseAlreadyExists).
			Message(fmt.Sprintf("VirtualMachineMACAddressLease %s is bound to another VirtualMachineMACAddress resource",
				lease.Name))

	case lease.Spec.VirtualMachineMACAddressRef.Namespace != mac.Namespace:
		macStatus.Phase = virtv2.VirtualMachineMACAddressPhasePending
		conditionBound.Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseAlreadyExists).
			Message(fmt.Sprintf("The VirtualMachineIPLease %s belongs to a different namespace", lease.Name))

		needRequeue = true

	default:
		if macStatus.Phase != virtv2.VirtualMachineMACAddressPhasePending {
			macStatus.Phase = virtv2.VirtualMachineMACAddressPhasePending
			conditionBound.Status(metav1.ConditionFalse).
				Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
				Message(fmt.Sprintf("VirtualMachineMACAddressLease %s is not ready",
					lease.Name))
		}
	}

	log.Debug("Set VirtualMachineMAC phase", "phase", macStatus.Phase)

	macStatus.ObservedGeneration = mac.GetGeneration()
	if !needRequeue {
		return reconcile.Result{}, nil
	} else {
		// TODO add requeue with with exponential BackOff time interval using condition Bound -> probeTime
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

func (h LifecycleHandler) Name() string {
	return LifecycleHandlerName
}
