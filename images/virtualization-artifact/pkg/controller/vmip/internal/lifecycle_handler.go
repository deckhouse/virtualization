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
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

const LifecycleHandlerName = "LifecycleHandler"

type LifecycleHandler struct {
	recorder eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		recorder: recorder,
	}
}

func (h *LifecycleHandler) Handle(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(LifecycleHandlerName))

	vmip := state.VirtualMachineIP()

	vm, err := state.VirtualMachine(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	conditionBound := conditions.NewConditionBuilder(vmipcondition.BoundType).
		Generation(vmip.GetGeneration()).
		Reason(conditions.ReasonUnknown).
		Status(metav1.ConditionUnknown)

	conditionAttach := conditions.NewConditionBuilder(vmipcondition.AttachedType).
		Generation(vmip.GetGeneration()).
		Reason(conditions.ReasonUnknown).
		Status(metav1.ConditionUnknown)

	defer func() {
		conditions.SetCondition(conditionBound, &vmip.Status.Conditions)
		conditions.SetCondition(conditionAttach, &vmip.Status.Conditions)
	}()

	lease, err := state.VirtualMachineIPLease(ctx)
	if err != nil {
		if err.Error() == "VirtualMachineIPAddressLease found in kubeclient without cache" {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	needRequeue := false
	switch {
	case lease == nil && vmip.Status.Address != "":
		h.handleLostLease(vmip, conditionBound)
	case lease == nil:
		h.handleNotFoundLease(vmip, conditionBound)
	default:
		needRequeue = h.handleBoundLease(lease, vmip, log, conditionBound)
		h.handleAttachedLease(vmip, vm, conditionAttach)
	}

	log.Debug("Set VirtualMachineIP phase", "phase", vmip.Status.Phase)
	vmip.Status.ObservedGeneration = vmip.GetGeneration()
	if !needRequeue {
		return reconcile.Result{}, nil
	} else {
		// TODO add requeue with with exponential BackOff time interval using condition Bound -> probeTime
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

func (h *LifecycleHandler) handleNotFoundLease(vmip *virtv2.VirtualMachineIPAddress, conditionBound *conditions.ConditionBuilder) {
	vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhasePending
	conditionBound.Status(metav1.ConditionFalse).
		Reason(vmipcondition.VirtualMachineIPAddressLeaseNotFound).
		Message("VirtualMachineIPAddressLease is not found")
}

func (h *LifecycleHandler) handleLostLease(vmip *virtv2.VirtualMachineIPAddress, conditionBound *conditions.ConditionBuilder) {
	vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhasePending
	conditionBound.Status(metav1.ConditionFalse).
		Reason(vmipcondition.VirtualMachineIPAddressLeaseLost).
		Message(fmt.Sprintf("VirtualMachineIPAddressLease %s doesn't exist",
			ip.IpToLeaseName(vmip.Status.Address)))
}

func (h *LifecycleHandler) handleAttachedLease(vmip *virtv2.VirtualMachineIPAddress, vm *virtv2.VirtualMachine, conditionAttach *conditions.ConditionBuilder) {
	if vm == nil || vm.DeletionTimestamp != nil {
		vmip.Status.VirtualMachine = ""
		conditionAttach.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineNotFound).
			Message("Virtual machine not found")
		h.recorder.Event(vmip, corev1.EventTypeWarning, virtv2.ReasonNotAttached, "Virtual machine not found.")
		return
	}

	vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhaseAttached
	vmip.Status.VirtualMachine = vm.Name
	conditionAttach.Status(metav1.ConditionTrue).
		Reason(vmipcondition.Attached)
	h.recorder.Eventf(vmip, corev1.EventTypeNormal, virtv2.ReasonAttached, "VirtualMachineIPAddress is attached to %q/%q.", vm.Namespace, vm.Name)
}

func (h *LifecycleHandler) handleBoundLease(lease *virtv2.VirtualMachineIPAddressLease, vmip *virtv2.VirtualMachineIPAddress, log *slog.Logger, conditionBound *conditions.ConditionBuilder) bool {
	needRequeue := false
	switch {
	case util.IsBoundLease(lease, vmip):
		vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhaseBound
		vmip.Status.Address = ip.LeaseNameToIP(lease.Name)
		conditionBound.Status(metav1.ConditionTrue).
			Reason(vmipcondition.Bound)

	case lease.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseBound:
		vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhasePending
		log.Warn(fmt.Sprintf("VirtualMachineIPAddressLease %s is bound to another VirtualMachineIPAddress resource: %s/%s",
			lease.Name, lease.Spec.VirtualMachineIPAddressRef.Name, lease.Spec.VirtualMachineIPAddressRef.Namespace))
		conditionBound.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseAlreadyExists).
			Message(fmt.Sprintf("VirtualMachineIPAddressLease %s is bound to another VirtualMachineIPAddress resource",
				lease.Name))

	case lease.Spec.VirtualMachineIPAddressRef.Namespace != vmip.Namespace:
		vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhasePending
		conditionBound.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseAlreadyExists).
			Message(fmt.Sprintf("The VirtualMachineIPLease %s belongs to a different namespace", lease.Name))
		needRequeue = true

	default:
		vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhasePending
		conditionBound.Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(fmt.Sprintf("VirtualMachineIPAddressLease %s is not ready",
				lease.Name))
	}

	return needRequeue
}

func (h *LifecycleHandler) Name() string {
	return LifecycleHandlerName
}
