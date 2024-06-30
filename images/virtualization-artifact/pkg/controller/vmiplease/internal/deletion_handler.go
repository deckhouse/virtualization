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

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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

func (h *DeletionHandler) Handle(ctx context.Context, state state.VMIPLeaseState) (reconcile.Result, error) {
	changed := state.VirtualMachineIPAddressLease().Changed()
	current := state.VirtualMachineIPAddressLease().Current()

	vmip, err := state.VirtualMachineIPAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmip == nil {
		switch current.Spec.ReclaimPolicy {
		case virtv2.VirtualMachineIPAddressReclaimPolicyDelete, "":
			h.logger.Info("VirtualMachineIP not found: remove this VMIPLease")

			return reconcile.Result{}, h.client.Delete(ctx, current)
		case virtv2.VirtualMachineIPAddressReclaimPolicyRetain:
			if current.Spec.ClaimRef != nil {
				h.logger.Info("VirtualMachineIP not found: remove this ref from the spec and retain VMIPLease")
				changed.Spec.ClaimRef = nil

				return reconcile.Result{}, h.client.Update(ctx, changed)
			}
		default:
			return reconcile.Result{}, fmt.Errorf("unexpected reclaimPolicy: %s", current.Spec.ReclaimPolicy)
		}
	}

	return reconcile.Result{}, nil
}
