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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

type ProtectionHandler struct{}

func NewProtectionHandler() *ProtectionHandler {
	return &ProtectionHandler{}
}

func (h *ProtectionHandler) Handle(_ context.Context, lease *virtv2.VirtualMachineIPAddressLease) (reconcile.Result, error) {
	controllerutil.AddFinalizer(lease, virtv2.FinalizerIPAddressLeaseCleanup)

	// 1. The lease has a finalizer throughout its lifetime to prevent it from being deleted without prior processing by the controller.
	if lease.GetDeletionTimestamp() == nil {
		return reconcile.Result{}, nil
	}

	// 2. It is necessary to protect the resource until we can unequivocally ensure that the resource is in the Released state.
	boundCondition, _ := conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
	if boundCondition.Reason != vmiplcondition.Released.String() || !conditions.IsLastUpdated(boundCondition, lease) {
		return reconcile.Result{}, nil
	}

	// 3. All checks have passed, the resource can be deleted.
	controllerutil.RemoveFinalizer(lease, virtv2.FinalizerIPAddressLeaseCleanup)
	return reconcile.Result{}, nil
}
