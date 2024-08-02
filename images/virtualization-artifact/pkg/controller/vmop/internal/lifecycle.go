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
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
)

const lifecycleHandlerName = "LifecycleHandler"

// LifecycleHandler calculate status of the VMOP resource.
type LifecycleHandler struct {
	logger *slog.Logger
}

func NewLifecycleHandler(logger *slog.Logger) *LifecycleHandler {
	return &LifecycleHandler{
		logger: logger.With("handler", lifecycleHandlerName),
	}
}

func (h LifecycleHandler) Handle(ctx context.Context, s state.VMOperationState) (reconcile.Result, error) {
	vmop := s.VMOP()
	if vmop == nil {
		return reconcile.Result{}, nil
	}

	changed := vmop.Changed()

	// Do not update conditions for object in the deletion state.
	if changed.DeletionTimestamp != nil {
		changed.Status.Phase = "Terminating"
		return reconcile.Result{}, nil
	}

	// TODO refactor old UpdateStatus here.
	//mgr := conditions.NewManager(changed.Status.Conditions)
	//conditionCompleted := conditions.NewConditionBuilder(vmopcondition.CompletedType).
	//	Generation(changed.GetGeneration())
	//conditionSignalSent := conditions.NewConditionBuilder(vmopcondition.SignalSentType).
	//	Generation(changed.GetGeneration())

	return reconcile.Result{}, nil
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}
