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

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ProtectionHandlerName = "ProtectionHandler"

type ProtectionHandler struct {
	logger logr.Logger
}

func NewProtectionHandler(logger logr.Logger) *ProtectionHandler {
	return &ProtectionHandler{
		logger: logger.WithValues("handler", ProtectionHandlerName),
	}
}

func (h *ProtectionHandler) Handle(ctx context.Context, state state.VMIPLeaseState) (reconcile.Result, error) {
	lease := state.VirtualMachineIPAddressLease()

	vmip, err := state.VirtualMachineIPAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmip != nil {
		controllerutil.AddFinalizer(lease, virtv2.FinalizerIPAddressLeaseCleanup)
	} else if lease.GetDeletionTimestamp() == nil {
		controllerutil.RemoveFinalizer(lease, virtv2.FinalizerIPAddressLeaseCleanup)
	}

	return reconcile.Result{}, nil
}

func (h *ProtectionHandler) Name() string {
	return ProtectionHandlerName
}
