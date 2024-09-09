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
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const protectionHandlerName = "ProtectionHandler"

// ProtectionHandler manages finalizers on VirtualMachineOperation resource.
type ProtectionHandler struct {
	logger *slog.Logger
	client client.Client
}

func NewProtectionHandler(logger *slog.Logger, client client.Client) *ProtectionHandler {
	return &ProtectionHandler{
		logger: logger.With("handler", protectionHandlerName),
		client: client,
	}
}

func (h ProtectionHandler) Handle(ctx context.Context, s state.VMOperationState) (reconcile.Result, error) {
	if s.VirtualMachineOperation() == nil {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachineOperation().Changed()
	log := h.logger.With("name", changed.GetName(), "namespace", changed.GetNamespace())

	vm, _ := s.VirtualMachine(ctx)

	// The only case when we need finalizer on VirtualMachineOperation is when operation is in progress.
	if changed.DeletionTimestamp == nil && changed.Status.Phase == virtv2.VMOPPhaseInProgress {
		log.Debug("Add protection finalizer for the InProgress phase")
		controllerutil.AddFinalizer(changed, virtv2.FinalizerVMOPCleanup)
		if vm != nil {
			return reconcile.Result{}, h.ensureVMFinalizers(ctx, vm)
		}
		return reconcile.Result{}, nil
	}

	// Remove finalizer when VirtualMachineOperation is in deletion state or not in progress.
	log.Debug(fmt.Sprintf("Unprotect VMOP: deletion %v, phase %s", changed.DeletionTimestamp != nil, changed.Status.Phase))
	controllerutil.RemoveFinalizer(changed, virtv2.FinalizerVMOPCleanup)
	if vm != nil {
		return reconcile.Result{}, h.removeVMFinalizers(ctx, vm)
	}
	return reconcile.Result{}, nil
}

func (h ProtectionHandler) Name() string {
	return protectionHandlerName
}

// ensureVMFinalizers ensures that VM has a finalizer.
// TODO refactor with patching.
func (h ProtectionHandler) ensureVMFinalizers(ctx context.Context, vm *virtv2.VirtualMachine) error {
	if vm != nil && controllerutil.AddFinalizer(vm, virtv2.FinalizerVMOPProtection) {
		if err := h.client.Update(ctx, vm); err != nil {
			return fmt.Errorf("error setting finalizer on a VM %q: %w", vm.Name, err)
		}
	}
	return nil
}

// removeVMFinalizers removes finalizer from VM.
// TODO refactor with patching.
func (h ProtectionHandler) removeVMFinalizers(ctx context.Context, vm *virtv2.VirtualMachine) error {
	if vm != nil && controllerutil.RemoveFinalizer(vm, virtv2.FinalizerVMOPProtection) {
		if err := h.client.Update(ctx, vm); err != nil {
			return fmt.Errorf("unable to remove VM %q finalizer %q: %w", vm.Name, virtv2.FinalizerVMOPProtection, err)
		}
	}
	return nil
}
