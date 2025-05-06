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

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

const RetentionHandlerName = "RetentionHandler"

type RetentionHandler struct {
	retentionDuration time.Duration
}

func NewRetentionHandler(retentionDuration time.Duration) *RetentionHandler {
	return &RetentionHandler{
		retentionDuration: retentionDuration,
	}
}

func (h *RetentionHandler) Handle(ctx context.Context, state state.VMIPLeaseState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(RetentionHandlerName))

	lease := state.VirtualMachineIPAddressLease()
	vmip, err := state.VirtualMachineIPAddress(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmip == nil || vmip.Status.Address != ip.LeaseNameToIP(lease.Name) {
		if lease.Spec.VirtualMachineIPAddressRef.Name != "" {
			log.Debug("VirtualMachineIP not found: remove this ref from the spec and retain VMIPLease")
			lease.Spec.VirtualMachineIPAddressRef.Name = ""
			return reconcile.Result{RequeueAfter: h.retentionDuration}, nil
		}

		leaseStatus := &lease.Status
		boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, leaseStatus.Conditions)
		if boundCondition.Reason == vmiplcondition.Released.String() {
			currentTime := time.Now().UTC()

			duration := currentTime.Sub(boundCondition.LastTransitionTime.Time)
			if duration >= h.retentionDuration {
				log.Info(fmt.Sprintf("Delete VMIPLease after %s of being not claimed", h.retentionDuration.String()))
				state.SetDeletion(true)
				return reconcile.Result{}, nil
			}

			return reconcile.Result{RequeueAfter: h.retentionDuration - duration}, nil
		}
	}

	return reconcile.Result{}, nil
}

func (h *RetentionHandler) Name() string {
	return RetentionHandlerName
}
