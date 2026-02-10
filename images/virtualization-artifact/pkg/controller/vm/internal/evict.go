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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameEvictHandler = "EvictHandler"

func NewEvictHandler() *EvictHandler {
	return &EvictHandler{}
}

type EvictHandler struct{}

func (h *EvictHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachine().Changed()
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil || kvvmi.Status.EvacuationNodeName == "" {
		conditions.RemoveCondition(vmcondition.TypeEvictionRequired, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	conditions.SetCondition(
		conditions.NewConditionBuilder(vmcondition.TypeEvictionRequired).
			Generation(changed.GetGeneration()).
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonEvictionRequired).
			Message("VirtualMachine should be evicted from current node or restarted."),
		&changed.Status.Conditions,
	)
	return reconcile.Result{}, nil
}

func (h *EvictHandler) Name() string {
	return nameEvictHandler
}
