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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaclcondition"
)

const DeletionHandlerName = "DeletionHandler"

type DeletionHandler struct {
	client client.Client
}

func NewDeletionHandler(client client.Client) *DeletionHandler {
	return &DeletionHandler{
		client: client,
	}
}

func (h *DeletionHandler) Handle(ctx context.Context, lease *v1alpha2.VirtualMachineMACAddressLease) (reconcile.Result, error) {
	boundCondition, _ := conditions.GetCondition(vmmaclcondition.BoundType, lease.Status.Conditions)
	if boundCondition.Status == metav1.ConditionFalse && conditions.IsLastUpdated(boundCondition, lease) {
		err := h.client.Delete(ctx, lease)
		if err != nil && !k8serrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("delete released lease: %w", err)
		}
	}

	return reconcile.Result{}, nil
}

func (h *DeletionHandler) Name() string {
	return DeletionHandlerName
}
