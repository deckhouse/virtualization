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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameSizePolicyHandler = "SizePolicyHandler"

func NewSizePolicyHandler(client client.Client) *SizePolicyHandler {
	return &SizePolicyHandler{
		service: service.NewSizePolicyService(),
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

	if update := addAllUnknown(changed, vmcondition.TypeSizingPolicyMatched); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	vmClass, err := s.Class(ctx)
	if err != nil && !errors.IsNotFound(err) {
		log := logger.FromContext(ctx)
		log.Error(
			"An error occurred while retrieving the VM class.",
			logger.SlogErr(err),
		)
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeSizingPolicyMatched).
		Generation(current.GetGeneration())

	switch {
	case vmClass == nil:
		cb.Message(fmt.Sprintf(" VirtualMachineClassName %q not found.", changed.Spec.VirtualMachineClassName)).
			Reason(vmcondition.ReasonVirtualMachineClassNotExists).
			Status(metav1.ConditionFalse)
	case vmClass.Status.Phase == v1alpha2.ClassPhaseTerminating:
		cb.Message(fmt.Sprintf("Virtual machine class %q is terminating.", vmClass.Name)).
			Reason(vmcondition.ReasonVirtualMachineClassTerminating).
			Status(metav1.ConditionFalse)
	default:
		err = h.service.CheckVMMatchedSizePolicy(changed, vmClass)
		if err == nil {
			cb.Reason(vmcondition.ReasonSizingPolicyMatched).
				Status(metav1.ConditionTrue)
		} else {
			cb.Message(fmt.Sprintf("Size policy matching errors: %s.", err.Error())).
				Reason(vmcondition.ReasonSizingPolicyNotMatched).
				Status(metav1.ConditionFalse)
		}
	}

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *SizePolicyHandler) Name() string {
	return nameSizePolicyHandler
}
