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
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

const retentionHandlerName = "RetentionHandler"

type RetentionHandler struct {
	retentionDuration time.Duration
	client            client.Client
}

func NewRetentionHandler(retentionDuration time.Duration, client client.Client) *RetentionHandler {
	return &RetentionHandler{
		retentionDuration: retentionDuration,
		client:            client,
	}
}

func (h *RetentionHandler) Handle(ctx context.Context, lease *virtv2.VirtualMachineIPAddressLease) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(retentionHandlerName))

	// Make sure that the Lease can be deleted only if it has already been verified that it is indeed Released.
	// Otherwise, we cannot guarantee that it is not still in use. In this case, it should remain and not be deleted.
	boundCondition, _ := conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
	if boundCondition.Reason == vmiplcondition.Released.String() && conditions.IsLastUpdated(boundCondition, lease) {
		currentTime := time.Now().UTC()

		duration := currentTime.Sub(boundCondition.LastTransitionTime.Time.UTC())
		if duration >= h.retentionDuration {
			log.Info(fmt.Sprintf("Released VirtualMachineIPAddressLease has not been used for more than %s. It will be deleted now.", h.retentionDuration.String()))

			err := h.client.Delete(ctx, lease)
			if err != nil && !k8serrors.IsNotFound(err) {
				return reconcile.Result{}, fmt.Errorf("delete released lease: %w", err)
			}

			return reconcile.Result{}, nil
		}

		return reconcile.Result{RequeueAfter: h.retentionDuration - duration}, nil
	}

	return reconcile.Result{}, nil
}
