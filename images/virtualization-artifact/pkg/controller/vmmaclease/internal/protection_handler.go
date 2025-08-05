/*
Copyright 2025 Flant JSC

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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaclcondition"
)

const ProtectionHandlerName = "ProtectionHandler"

type ProtectionHandler struct{}

func NewProtectionHandler() *ProtectionHandler {
	return &ProtectionHandler{}
}

func (h *ProtectionHandler) Handle(ctx context.Context, lease *virtv2.VirtualMachineMACAddressLease) (reconcile.Result, error) {
	controllerutil.AddFinalizer(lease, virtv2.FinalizerMACAddressLeaseCleanup)

	// 1. The lease has a finalizer throughout its lifetime to prevent it from being deleted without prior processing by the controller.
	if lease.GetDeletionTimestamp() == nil {
		return reconcile.Result{}, nil
	}

	// 2. It is necessary to protect the resource until we can unequivocally ensure that the resource is in not Bound state.
	boundCondition, _ := conditions.GetCondition(vmmaclcondition.BoundType, lease.Status.Conditions)
	if boundCondition.Status != metav1.ConditionFalse || !conditions.IsLastUpdated(boundCondition, lease) {
		return reconcile.Result{}, nil
	}

	// 3. All checks have passed, the resource can be deleted.
	controllerutil.RemoveFinalizer(lease, virtv2.FinalizerMACAddressLeaseCleanup)
	return reconcile.Result{}, nil
}

func (h *ProtectionHandler) Name() string {
	return ProtectionHandlerName
}
