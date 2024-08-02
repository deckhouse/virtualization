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

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const protectionHandlerName = "ProtectionHandler"

// ProtectionHandler manages finalizers on VMOP resource.
type ProtectionHandler struct {
	logger *slog.Logger
}

func NewProtectionHandler(logger *slog.Logger) *ProtectionHandler {
	return &ProtectionHandler{
		logger: logger.With("handler", protectionHandlerName),
	}
}

func (h ProtectionHandler) Handle(_ context.Context, s state.VMOperationState) (reconcile.Result, error) {
	if s.VMOP() == nil {
		return reconcile.Result{}, nil
	}

	changed := s.VMOP().Changed()
	log := h.logger.With("name", changed.GetName(), "namespace", changed.GetNamespace())
	if changed.DeletionTimestamp != nil {
		log.Debug("Observe VMOP in deletion state, remove finalizer")
		controllerutil.RemoveFinalizer(changed, virtv2.FinalizerVMOPCleanup)
		return reconcile.Result{}, nil
	}

	// TODO check conditions for other cases when to remove finalizer:
	// case state.IsCompleted():
	//		log.V(2).Info("VMOP completed", "namespacedName", req.String())
	//		return r.removeFinalizers(ctx, state, opts)
	//
	//	case state.IsFailed():
	//		log.V(2).Info("VMOP failed", "namespacedName", req.String())
	//		return r.removeFinalizers(ctx, state, opts)

	controllerutil.AddFinalizer(changed, virtv2.FinalizerVMOPCleanup)
	return reconcile.Result{}, nil
}

func (h ProtectionHandler) Name() string {
	return protectionHandlerName
}
