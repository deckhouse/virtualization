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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1alpha1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameQuotaExceedHandler = "QuotaExceedHandler"

func NewQuotaExceedHandler() *QuotaExceedHandler {
	return &QuotaExceedHandler{}
}

type QuotaExceedHandler struct{}

func (h *QuotaExceedHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeQuotaNotExceeded).Generation(changed.GetGeneration())
	defer func() {
		conditions.SetCondition(cb, &changed.Status.Conditions)
	}()

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil {
		cb.
			Reason(vmcondition.ReasonKVVMINotExists).
			Message("KVVMI is not exist")
		return reconcile.Result{}, nil
	}

	var kvvmiSynchronizedCondition v1alpha1.VirtualMachineInstanceCondition
	for _, c := range kvvmi.Status.Conditions {
		if c.Type == "Synchronized" {
			kvvmiSynchronizedCondition = c
		}
	}

	if strings.Contains(kvvmiSynchronizedCondition.Message, "exceeded quota") {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonQuotaExceeded).
			Message(kvvmiSynchronizedCondition.Message)
	} else {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonQuotaNotExceeded).
			Message("")
	}

	return reconcile.Result{}, nil
}

func (h *QuotaExceedHandler) Name() string {
	return nameQuotaExceedHandler
}
