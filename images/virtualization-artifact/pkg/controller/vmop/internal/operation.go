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
	"time"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
)

const operationHandlerName = "OperationHandler"

// OperationHandler performs operation on Virtual Machine.
type OperationHandler struct {
	logger *slog.Logger
}

func NewOperationHandler(logger *slog.Logger) *OperationHandler {
	return &OperationHandler{
		logger: logger,
	}
}

func (h OperationHandler) Handle(ctx context.Context, s state.VMOperationState) (reconcile.Result, error) {
	// Requeue if there is other VMOP in progress.
	found, err := s.OtherVMOPIsInProgress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if found {
		// TODO Add the Completed condition with the WaitingReason.

		// TODO can we replace requeue with watcher settings?
		return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
	}

	return reconcile.Result{}, nil
}

func (h OperationHandler) Name() string {
	return operationHandlerName
}
