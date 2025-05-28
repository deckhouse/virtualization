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
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/blockdevice"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/step"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

type BoundHandler struct {
	ipService IPAddressService
	client    client.Client
}

func NewBoundHandler(ipService IPAddressService, client client.Client) *BoundHandler {
	return &BoundHandler{
		ipService: ipService,
		client:    client,
	}
}

func (h *BoundHandler) Handle(ctx context.Context, vmip *virtv2.VirtualMachineIPAddress) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmipcondition.BoundType).Generation(vmip.Generation)
	defer func() { conditions.SetCondition(cb, &vmip.Status.Conditions) }()

	lease, err := h.ipService.GetLease(ctx, vmip)
	if err != nil {
		err = fmt.Errorf("error occured: %w", err)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")

		if k8serrors.IsTooManyRequests(err) {
			logger.FromContext(ctx).Warn(err.Error())
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		}

		return reconcile.Result{}, err
	}

	if lease != nil {
		log := logger.FromContext(ctx).With("leaseName", lease.Name)
		ctx = logger.ToContext(ctx, log)
	}

	return blockdevice.NewStepTakers[*virtv2.VirtualMachineIPAddress](
		step.NewBindStep(lease, cb),
		step.NewTakeLeaseStep(lease, h.client, cb),
		step.NewCreateLeaseStep(lease, h.ipService, h.client, cb),
	).Run(ctx, vmip)
}
