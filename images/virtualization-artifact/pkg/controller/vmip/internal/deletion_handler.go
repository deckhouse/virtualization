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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
)

type DeletionHandler struct {
	client client.Client
	logger logr.Logger
}

func NewDeletionHandler(client client.Client, logger logr.Logger) *DeletionHandler {
	return &DeletionHandler{
		client: client,
		logger: logger.WithValues("handler", "DeletionHandler"),
	}
}

func (h *DeletionHandler) Handle(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	vm, err := state.VirtualMachine(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vm == nil && state.VirtualMachineIP().Current().Labels[common.LabelImplicitIPAddressClaim] == common.LabelImplicitIPAddressClaimValue {
		h.logger.Info("The VirtualMachineIP is implicit: delete it", "name", state.VirtualMachineIP().Name())
		return reconcile.Result{}, h.client.Delete(ctx, state.VirtualMachineIP().Current())
	}

	return reconcile.Result{}, nil
}
