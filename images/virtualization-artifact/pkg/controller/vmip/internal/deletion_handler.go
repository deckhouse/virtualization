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
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

type DeletionHandler struct {
	client client.Client
}

func NewDeletionHandler(client client.Client) *DeletionHandler {
	return &DeletionHandler{
		client: client,
	}
}

func (h *DeletionHandler) Handle(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (reconcile.Result, error) {
	attachedCondition, _ := conditions.GetCondition(vmipcondition.AttachedType, vmip.Status.Conditions)
	if attachedCondition.Status == metav1.ConditionTrue || !conditions.IsLastUpdated(attachedCondition, vmip) {
		return reconcile.Result{}, nil
	}

	// This is done to allow new resources to be created while ensuring they are deleted when the interface is removed from the virtual machine's specification.
	diff := vmip.CreationTimestamp.Time.Sub(attachedCondition.LastTransitionTime.Time).Abs()
	if diff.Seconds() < 1 {
		return reconcile.Result{}, nil
	}

	err := h.client.Delete(ctx, vmip)
	if err != nil && !k8serrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("delete vmip: %w", err)
	}

	return reconcile.Result{}, nil
}
