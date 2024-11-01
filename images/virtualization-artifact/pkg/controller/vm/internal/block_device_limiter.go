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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameBlockDeviceLimiterHandler = "BlockDeviceLimiterHandler"

type BlockDeviceLimiterHandler struct {
	service *service.BlockDeviceService
}

func NewBlockDeviceLimiterHandler(service *service.BlockDeviceService) *BlockDeviceLimiterHandler {
	return &BlockDeviceLimiterHandler{service: service}
}

func (h *BlockDeviceLimiterHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if isDeletion(current) {
		changed.Status.Phase = virtv2.MachineTerminating
		return reconcile.Result{}, nil
	}

	if updated := addAllUnknown(changed, vmcondition.TypeDiskAttachmentCapacityAvailable); updated || changed.Status.Phase == "" {
		changed.Status.Phase = virtv2.MachinePending
		return reconcile.Result{Requeue: true}, nil
	}

	blockDeviceAttachedCount, err := h.service.CountBlockDevicesAttachedToVm(ctx, changed)
	if err != nil {
		return reconcile.Result{}, err
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeDiskAttachmentCapacityAvailable).Generation(changed.Generation)
	defer func() { conditions.SetCondition(cb, &changed.Status.Conditions) }()

	if blockDeviceAttachedCount > common.VmBlockDeviceAttachedLimit {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonBlockDeviceCapacityAvailable).
			Message("")
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonBlockDeviceCapacityReached).
			Message(fmt.Sprintf("Can not attach %d block devices (%d is maximum) to `VirtualMachine` %q", blockDeviceAttachedCount, common.VmBlockDeviceAttachedLimit, changed.Name))
	}

	return reconcile.Result{}, nil
}

func (h *BlockDeviceLimiterHandler) Name() string {
	return nameBlockDeviceLimiterHandler
}
