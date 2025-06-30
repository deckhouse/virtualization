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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

type LifecycleHandler struct {
	recorder eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		recorder: recorder,
	}
}

func (h *LifecycleHandler) Handle(_ context.Context, vmip *virtv2.VirtualMachineIPAddress) (reconcile.Result, error) {
	boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, vmip.Status.Conditions)
	if boundCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(boundCondition, vmip) {
		vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhasePending
		return reconcile.Result{}, nil
	}

	attachedCondition, _ := conditions.GetCondition(vmipcondition.AttachedType, vmip.Status.Conditions)
	if attachedCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(boundCondition, vmip) {
		if vmip.Status.Phase != virtv2.VirtualMachineIPAddressPhaseBound {
			h.recorder.Eventf(vmip, corev1.EventTypeNormal, virtv2.ReasonBound, "VirtualMachineIPAddress is bound.")
		}
		vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhaseBound
		return reconcile.Result{}, nil
	}

	vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhaseAttached
	return reconcile.Result{}, nil
}
