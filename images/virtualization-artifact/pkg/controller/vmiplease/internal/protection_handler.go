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

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ProtectionHandlerName = "ProtectionHandler"

type ProtectionHandler struct{}

func NewProtectionHandler() *ProtectionHandler {
	return &ProtectionHandler{}
}

func (h *ProtectionHandler) Handle(ctx context.Context, state state.VMIPLeaseState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(ProtectionHandlerName))
	lease := state.VirtualMachineIPAddressLease()

	vmip, err := state.VirtualMachineIPAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmip != nil {
		controllerutil.AddFinalizer(lease, virtv2.FinalizerIPAddressLeaseCleanup)
	} else {
		log.Info("Deletion observed: remove cleanup finalizer from VirtualMachineIPAddressLease")
		controllerutil.RemoveFinalizer(lease, virtv2.FinalizerIPAddressLeaseCleanup)
	}

	return reconcile.Result{}, nil
}

func (h *ProtectionHandler) Name() string {
	return ProtectionHandlerName
}
