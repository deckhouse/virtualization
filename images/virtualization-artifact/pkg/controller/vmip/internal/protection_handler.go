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

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ProtectionHandlerName = "ProtectionHandler"

type ProtectionHandler struct{}

func NewProtectionHandler() *ProtectionHandler {
	return &ProtectionHandler{}
}

func (h *ProtectionHandler) Handle(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(ProtectionHandlerName))

	vmip := state.VirtualMachineIP()

	vm, err := state.VirtualMachine(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vm == nil || vm.DeletionTimestamp != nil {
		log.Info("VirtualMachineIP is no longer attached to any VM, proceeding with detachment", "VirtualMachineIPName", vmip.Name)
		controllerutil.RemoveFinalizer(vmip, virtv2.FinalizerIPAddressCleanup)
	} else if vmip.GetDeletionTimestamp() == nil {
		controllerutil.AddFinalizer(vmip, virtv2.FinalizerIPAddressCleanup)
		log.Info("VirtualMachineIP is still attached, finalizer added", "VirtualMachineIPName", vmip.Name)
	}

	return reconcile.Result{}, nil
}

func (h *ProtectionHandler) Name() string {
	return ProtectionHandlerName
}
