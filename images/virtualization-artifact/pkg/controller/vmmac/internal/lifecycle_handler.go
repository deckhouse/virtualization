/*
Copyright 2025 Flant JSC

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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

type LifecycleHandler struct {
	recorder eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		recorder: recorder,
	}
}

func (h *LifecycleHandler) Handle(_ context.Context, vmmac *virtv2.VirtualMachineMACAddress) (reconcile.Result, error) {
	boundCondition, _ := conditions.GetCondition(vmmaccondition.BoundType, vmmac.Status.Conditions)
	if boundCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(boundCondition, vmmac) {
		vmmac.Status.Phase = virtv2.VirtualMachineMACAddressPhasePending
		return reconcile.Result{}, nil
	}

	attachedCondition, _ := conditions.GetCondition(vmmaccondition.AttachedType, vmmac.Status.Conditions)
	if attachedCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(boundCondition, vmmac) {
		if vmmac.Status.Phase != virtv2.VirtualMachineMACAddressPhaseBound {
			h.recorder.Eventf(vmmac, corev1.EventTypeNormal, virtv2.ReasonBound, "VirtualMachineMACAddress is bound.")
		}
		vmmac.Status.Phase = virtv2.VirtualMachineMACAddressPhaseBound
		return reconcile.Result{}, nil
	}

	vmmac.Status.Phase = virtv2.VirtualMachineMACAddressPhaseAttached
	return reconcile.Result{}, nil
}
