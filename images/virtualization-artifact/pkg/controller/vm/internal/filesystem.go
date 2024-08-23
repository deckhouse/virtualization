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
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameFilesystemHandler = "FilesystemHandler"

var filesystemConditions = []string{
	string(vmcondition.TypeFilesystemReady),
}

func NewFilesystemHandler() *FilesystemHandler {
	return &FilesystemHandler{}
}

type FilesystemHandler struct{}

func (h *FilesystemHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, filesystemConditions...); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(changed) {
		return reconcile.Result{}, nil
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeFilesystemReady).Generation(changed.GetGeneration())
	defer func() { mgr.Update2(cb); changed.Status.Conditions = mgr.Generate() }()

	if kvvmi == nil {
		cb.Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonFilesystemNotReady).
			Message(fmt.Sprintf("The internal virtual machine %s is not running.", changed.Name))
		return reconcile.Result{}, nil
	}

	agentReady, _ := service.GetCondition(vmcondition.TypeAgentReady.String(), changed.Status.Conditions)
	if agentReady.Status != metav1.ConditionTrue {
		cb.Status(metav1.ConditionUnknown)
		return reconcile.Result{}, nil
	}

	if kvvmi.Status.FSFreezeStatus == "frozen" {
		cb.Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonFilesystemFrozen).
			Message(fmt.Sprintf("The internal virtual machine %s is frozen.", changed.Name))
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionTrue).
		Reason2(vmcondition.ReasonFilesystemReady)
	return reconcile.Result{}, nil
}

func (h *FilesystemHandler) Name() string {
	return nameFilesystemHandler
}
