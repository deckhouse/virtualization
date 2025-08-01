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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

type ProtectionHandler struct{}

func NewProtectionHandler() *ProtectionHandler {
	return &ProtectionHandler{}
}

func (h *ProtectionHandler) Handle(_ context.Context, vmip *virtv2.VirtualMachineIPAddress) (reconcile.Result, error) {
	controllerutil.AddFinalizer(vmip, virtv2.FinalizerIPAddressCleanup)

	// 1. The vmip has a finalizer throughout its lifetime to prevent it from being deleted without prior processing by the controller.
	if vmip.GetDeletionTimestamp() == nil {
		return reconcile.Result{}, nil
	}

	// 2. It is necessary to keep vmip protected until we can unequivocally ensure that the resource is not in the Attached state.
	attachedCondition, _ := conditions.GetCondition(vmipcondition.AttachedType, vmip.Status.Conditions)
	if attachedCondition.Status == metav1.ConditionTrue || !conditions.IsLastUpdated(attachedCondition, vmip) {
		return reconcile.Result{}, nil
	}

	// 3. All checks have passed, the resource can be deleted.
	controllerutil.RemoveFinalizer(vmip, virtv2.FinalizerIPAddressCleanup)

	// 4. Remove legacy finalizer as well. It no longer attaches to new resources, but must be removed from old ones.
	controllerutil.RemoveFinalizer(vmip, virtv2.FinalizerIPAddressProtection)

	return reconcile.Result{}, nil
}
