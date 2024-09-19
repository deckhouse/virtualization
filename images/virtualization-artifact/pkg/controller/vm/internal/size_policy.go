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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const nameSizePolicyHandler = "SizePolicyHandler"

func NewSizePolicyHandler(client client.Client) *SizePolicyHandler {
	return &SizePolicyHandler{
		service: service.NewSizePolicyService(client),
	}
}

type SizePolicyHandler struct {
	service *service.SizePolicyService
}

func (h *SizePolicyHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, agentConditions...); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	//nolint:staticcheck
	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder(vmcondition.TypeSizingPolicyMatched).
		Generation(current.GetGeneration())

	err := h.service.CheckVMMatchedSizePolicy(ctx, changed)
	if err == nil {
		cb.Message("").
			Reason(vmcondition.ReasonSizingPolicyMatched).
			Status(metav1.ConditionTrue)
	} else {
		cb.Message(err.Error()).
			Reason(vmcondition.ReasonSizingPolicyMatched).
			Status(metav1.ConditionFalse)
	}

	mgr.Update(cb.Condition())
	changed.Status.Conditions = mgr.Generate()

	return reconcile.Result{}, nil
}

func (h *SizePolicyHandler) Name() string {
	return nameSizePolicyHandler
}
