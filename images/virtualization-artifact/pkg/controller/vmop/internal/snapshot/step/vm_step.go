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

package step

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StopVMStep struct {
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
	vm       *virtv2.VirtualMachine
}

func NewStopVMStep(
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	vm *virtv2.VirtualMachine,
) *StopVMStep {
	return &StopVMStep{
		recorder: recorder,
		cb:       cb,
		vm:       vm,
	}
}

func (s StopVMStep) Take(ctx context.Context, vm *virtv2.VirtualMachine) (*reconcile.Result, error) {
	// log, _ := logger.GetDataSourceContext(ctx, "objectref")

	return &reconcile.Result{}, nil
}
