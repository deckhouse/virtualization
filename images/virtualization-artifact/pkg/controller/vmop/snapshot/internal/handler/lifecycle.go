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

package handler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

type Base interface {
	Init(vmop *v1alpha2.VirtualMachineOperation)
	ShouldExecuteOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error)
	FetchVirtualMachineOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*v1alpha2.VirtualMachine, error)
	IsApplicableOrSetFailedPhase(checker genericservice.ApplicableChecker, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) bool
}

type LifecycleHandler struct {
	svcOpCreator SvcOpCreator
	base         Base
	recorder     eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(svcOpCreator SvcOpCreator, base Base, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		svcOpCreator: svcOpCreator,
		base:         base,
		recorder:     recorder,
	}
}

// Handle sets conditions depending on cluster state.
// It should set Running condition to start operation on VM.
func (h LifecycleHandler) Handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	// Do not update conditions for object in the deletion state.
	if commonvmop.IsTerminating(vmop) {
		vmop.Status.Phase = v1alpha2.VMOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	// Ignore if VMOP is in final state or failed.
	if vmop.Status.Phase == v1alpha2.VMOPPhaseCompleted || vmop.Status.Phase == v1alpha2.VMOPPhaseFailed {
		return reconcile.Result{}, nil
	}

	completed, completedFound := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	if completedFound && completed.Status == metav1.ConditionTrue {
		vmop.Status.Phase = v1alpha2.VMOPPhaseCompleted
		return reconcile.Result{}, nil
	}

	svcOp, err := h.svcOpCreator(vmop)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 1.Initialize new VMOP resource: set phase to Pending and all conditions to Unknown.
	h.base.Init(vmop)

	// 2. Get VirtualMachine for validation vmop.
	vm, err := h.base.FetchVirtualMachineOrSetFailedPhase(ctx, vmop)
	if vm == nil || err != nil {
		return reconcile.Result{}, err
	}

	// 3. Operation already in progress. Check if the operation is completed.
	// Run execute until the operation is completed.
	if _, found := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions); found {
		return svcOp.Execute(ctx)
	}

	// 4. VMOP is not in progress.
	// All operations must be performed in course, check it and set phase if operation cannot be executed now.
	should, err := h.base.ShouldExecuteOrSetFailedPhase(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !should {
		return reconcile.Result{}, nil
	}

	// 5. Check if the operation is applicable for executed.
	isApplicable := h.base.IsApplicableOrSetFailedPhase(svcOp, vmop, vm)
	if !isApplicable {
		return reconcile.Result{}, nil
	}

	// 6. The Operation is valid, and can be executed.
	vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
	return svcOp.Execute(ctx)
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}
