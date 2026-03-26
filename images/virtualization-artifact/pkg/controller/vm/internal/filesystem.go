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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

const (
	nameFilesystemHandler = "FilesystemHandler"
	annKek                = "kek"
)

func NewFilesystemHandler(virtClient VirtClient) *FilesystemHandler {
	return &FilesystemHandler{virtClient: virtClient}
}

type FilesystemHandler struct {
	virtClient VirtClient
}

func (h *FilesystemHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachine().Changed()

	if isDeletion(changed) {
		return reconcile.Result{}, nil
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeFilesystemFrozen).
		Status(metav1.ConditionUnknown).
		Generation(changed.GetGeneration())

	defer func() {
		if cb.Condition().Status == metav1.ConditionTrue {
			conditions.SetCondition(cb, &changed.Status.Conditions)
		} else {
			conditions.RemoveCondition(vmcondition.TypeFilesystemFrozen, &changed.Status.Conditions)
		}
	}()

	if kvvmi == nil {
		return reconcile.Result{}, nil
	}

	if request, ok := changed.Annotations[annKek]; ok {
		switch request {
		case "freeze":
			if err = h.virtClient.VirtualMachines(changed.Namespace).Freeze(ctx, changed.Name, subv1alpha2.VirtualMachineFreeze{}); err != nil {
				return reconcile.Result{}, fmt.Errorf("freeze virtual machine %s/%s: %w", changed.Namespace, changed.Name, err)
			}
		case "unfreeze":
			if err = h.virtClient.VirtualMachines(changed.Namespace).Unfreeze(ctx, changed.Name); err != nil {
				return reconcile.Result{}, fmt.Errorf("unfreeze virtual machine %s/%s: %w", changed.Namespace, changed.Name, err)
			}
		}
		delete(changed.Annotations, annKek)
	}

	agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, changed.Status.Conditions)
	if agentReady.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	if kvvmi.Status.FSFreezeStatus == "frozen" {
		cb.Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonFilesystemFrozen).
			Message("File system of the Virtual Machine is frozen.")
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (h *FilesystemHandler) Name() string {
	return nameFilesystemHandler
}
