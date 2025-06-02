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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

type BlockDeviceLimiter struct {
	service *service.BlockDeviceService
}

func NewBlockDeviceLimiter(service *service.BlockDeviceService) *BlockDeviceLimiter {
	return &BlockDeviceLimiter{service: service}
}

func (h *BlockDeviceLimiter) Handle(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	blockDeviceAttachedCount, err := h.service.CountBlockDevicesAttachedToVmName(ctx, vmbda.Spec.VirtualMachineName, vmbda.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	cb := conditions.NewConditionBuilder(vmbdacondition.DiskAttachmentCapacityAvailableType).Generation(vmbda.Generation)
	defer func() { conditions.SetCondition(cb, &vmbda.Status.Conditions) }()

	if vmbda.DeletionTimestamp != nil {
		cb.Status(metav1.ConditionUnknown).Reason(vmbdacondition.CapacityUnknown)
		return reconcile.Result{}, nil
	}

	if blockDeviceAttachedCount <= common.VMBlockDeviceAttachedLimit {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmbdacondition.CapacityAvailable).
			Message("")
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmbdacondition.CapacityReached).
			Message(fmt.Sprintf("Can not attach %d block devices (%d is maximum) to `VirtualMachine` %q", blockDeviceAttachedCount, common.VMBlockDeviceAttachedLimit, vmbda.Spec.VirtualMachineName))
	}

	return reconcile.Result{}, nil
}
