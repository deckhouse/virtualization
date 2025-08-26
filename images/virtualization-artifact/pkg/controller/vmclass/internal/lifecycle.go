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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmclasscondition"
)

const nameLifeCycleHandler = "LifeCycleHandler"

func NewLifeCycleHandler(client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		client: client,
	}
}

type LifeCycleHandler struct {
	client client.Client
}

func (h *LifeCycleHandler) Handle(_ context.Context, s state.VirtualMachineClassState) (reconcile.Result, error) {
	if s.VirtualMachineClass().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachineClass().Current()
	changed := s.VirtualMachineClass().Changed()
	if isDeletion(current) {
		changed.Status.Phase = v1alpha2.ClassPhaseTerminating
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmclasscondition.TypeReady).
		Generation(current.GetGeneration())
	var phase v1alpha2.VirtualMachineClassPhase

	switch current.Spec.CPU.Type {
	case v1alpha2.CPUTypeHostPassthrough, v1alpha2.CPUTypeHost:
		cb.Message("").
			Reason(vmclasscondition.ReasonSuitableNodesFound).
			Status(metav1.ConditionTrue)
		phase = v1alpha2.ClassPhaseReady
	case v1alpha2.CPUTypeDiscovery:
		var notReady bool
		if len(changed.Status.AvailableNodes) == 0 {
			cb.Message("No matching nodes found.")
			cb.Reason(vmclasscondition.ReasonNoSuitableNodesFound)
			notReady = true
		}
		if len(changed.Status.CpuFeatures.Enabled) == 0 {
			cb.Message("No cpu feature enabled.")
			cb.Reason(vmclasscondition.ReasonNoCpuFeaturesEnabled)
			notReady = true
		}
		if notReady {
			phase = v1alpha2.ClassPhasePending
			cb.Status(metav1.ConditionFalse)
			break
		}
		phase = v1alpha2.ClassPhaseReady
		cb.Message("").
			Reason(vmclasscondition.ReasonSuitableNodesFound).
			Status(metav1.ConditionTrue)
	default:
		if len(changed.Status.AvailableNodes) == 0 {
			phase = v1alpha2.ClassPhasePending
			cb.Message("No matching nodes found.").
				Reason(vmclasscondition.ReasonNoSuitableNodesFound).
				Status(metav1.ConditionFalse)
			break
		}
		phase = v1alpha2.ClassPhaseReady
		cb.Message("").
			Reason(vmclasscondition.ReasonSuitableNodesFound).
			Status(metav1.ConditionTrue)
	}
	conditions.SetCondition(cb, &changed.Status.Conditions)
	changed.Status.Phase = phase

	return reconcile.Result{}, nil
}

func (h *LifeCycleHandler) Name() string {
	return nameLifeCycleHandler
}
