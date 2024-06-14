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

package vmop

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ReconcilerState struct {
	Client client.Client
	Result *reconcile.Result

	VMOP *helper.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
	VM   *virtv2.VirtualMachine

	operationResult *OperationResult
}

type OperationResult struct {
	success bool
	message string
}

func (op *OperationResult) WasSuccessful() bool {
	return op.success
}

func (op *OperationResult) Message() string {
	return op.message
}

func NewReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *ReconcilerState {
	state := &ReconcilerState{
		Client: client,
		VMOP: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachineOperation { return &virtv2.VirtualMachineOperation{} },
			func(obj *virtv2.VirtualMachineOperation) virtv2.VirtualMachineOperationStatus { return obj.Status },
		),
	}
	return state
}

func (state *ReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	err := state.VMOP.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if state.VMOP.IsEmpty() {
		log.Info("Reconcile observe an absent VMOP: it may be deleted", "vmop.name", req.Name, "vmop.namespace", req.Namespace)
		return nil
	}

	vmName := state.VMOP.Current().Spec.VirtualMachine
	vm, err := helper.FetchObject(ctx,
		types.NamespacedName{Name: vmName, Namespace: req.Namespace},
		client,
		&virtv2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("unable to get VM %q: %w", vmName, err)
	}
	state.VM = vm

	return nil
}

func (state *ReconcilerState) ShouldReconcile(_ logr.Logger) bool {
	return !state.VMOP.IsEmpty()
}

func (state *ReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.VMOP.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VMOP %q meta: %w", state.VMOP.Name(), err)
	}
	return nil
}

func (state *ReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.VMOP.UpdateStatus(ctx)
}

func (state *ReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *ReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *ReconcilerState) IsDeletion() bool {
	if state.VMOP.IsEmpty() {
		return false
	}
	if !state.VmIsEmpty() && state.VM.DeletionTimestamp != nil {
		return true
	}
	return state.VMOP.Current().DeletionTimestamp != nil && !state.IsInProgress()
}

func (state *ReconcilerState) IsProtected() bool {
	return controllerutil.ContainsFinalizer(state.VMOP.Current(), virtv2.FinalizerVMOPCleanup)
}

func (state *ReconcilerState) IsCompleted() bool {
	if state.VMOP.IsEmpty() {
		return false
	}
	return state.VMOP.Current().Status.Phase == virtv2.VMOPPhaseCompleted
}

func (state *ReconcilerState) IsFailed() bool {
	if state.VMOP.IsEmpty() {
		return false
	}
	return state.VMOP.Current().Status.Phase == virtv2.VMOPPhaseFailed
}

func (state *ReconcilerState) IsInProgress() bool {
	if state.VMOP.IsEmpty() {
		return false
	}
	return state.VMOP.Current().Status.Phase == virtv2.VMOPPhaseInProgress
}

func (state *ReconcilerState) VmIsEmpty() bool {
	return state.VM == nil
}

func (state *ReconcilerState) OtherVMOPInProgress(ctx context.Context) (bool, error) {
	var vmops virtv2.VirtualMachineOperationList
	err := state.Client.List(ctx, &vmops, client.InNamespace(state.VMOP.Current().GetNamespace()))
	if err != nil {
		return false, err
	}
	vmName := state.VMOP.Current().Spec.VirtualMachine

	for _, vmop := range vmops.Items {
		if vmop.GetName() == state.VMOP.Current().GetName() || vmop.Spec.VirtualMachine != vmName {
			continue
		}
		if vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
			return true, nil
		}
	}
	return false, nil
}

func (state *ReconcilerState) SetOperationResult(result bool, msg string) {
	state.operationResult = &OperationResult{message: msg, success: result}
}

func (state *ReconcilerState) GetOperationResult() *OperationResult {
	return state.operationResult
}

func (state *ReconcilerState) GetKVVM(ctx context.Context) (*virtv1.VirtualMachine, error) {
	if state.VmIsEmpty() {
		return nil, fmt.Errorf("VM %s not found", state.VMOP.Current().Spec.VirtualMachine)
	}
	kvvm := &virtv1.VirtualMachine{}
	key := types.NamespacedName{Name: state.VM.GetName(), Namespace: state.VM.GetNamespace()}
	err := state.Client.Get(ctx, key, kvvm)
	return kvvm, err
}

func (state *ReconcilerState) GetKVVMI(ctx context.Context) (*virtv1.VirtualMachineInstance, error) {
	if state.VmIsEmpty() {
		return nil, fmt.Errorf("VM %s not found", state.VMOP.Current().Spec.VirtualMachine)
	}
	kvvmi := &virtv1.VirtualMachineInstance{}
	key := types.NamespacedName{Name: state.VM.GetName(), Namespace: state.VM.GetNamespace()}
	err := state.Client.Get(ctx, key, kvvmi)
	return kvvmi, err
}
