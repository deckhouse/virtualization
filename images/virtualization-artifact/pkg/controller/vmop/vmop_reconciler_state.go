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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ReconcilerState struct {
	Client client.Client
	Result *reconcile.Result

	VMOP *helper.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
	VM   *virtv2.VirtualMachine
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
