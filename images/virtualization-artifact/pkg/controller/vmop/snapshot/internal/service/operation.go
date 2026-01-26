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

package service

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type Operation interface {
	Execute(ctx context.Context) (reconcile.Result, error)
	IsApplicableForVMPhase(phase v1alpha2.MachinePhase) bool
	IsApplicableForRunPolicy(runPolicy v1alpha2.RunPolicy) bool
	GetInProgressReason() vmopcondition.ReasonCompleted
}

func NewOperationService(client client.Client, recorder eventrecord.EventRecorderLogger, vmop *v1alpha2.VirtualMachineOperation) (Operation, error) {
	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeRestore:
		return NewRestoreOperation(client, recorder, vmop), nil
	case v1alpha2.VMOPTypeClone:
		return NewCloneOperation(client, recorder, vmop), nil
	default:
		return nil, fmt.Errorf("unknown virtual machine operation type: %v", vmop.Spec.Type)
	}
}
