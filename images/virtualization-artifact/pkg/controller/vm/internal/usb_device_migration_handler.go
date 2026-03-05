/*
Copyright 2026 Flant JSC

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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameUSBDeviceMigrationHandler = "USBDeviceMigrationHandler"

// USBDeviceMigrationHandler unplugs all attached USB devices when migration is pending.
// After migration completes, the normal attach handler will plug them back (DRA over network).
func NewUSBDeviceMigrationHandler(cl client.Client, virtClient VirtClient) *USBDeviceMigrationHandler {
	return &USBDeviceMigrationHandler{
		usbDeviceHandlerBase: usbDeviceHandlerBase{
			client:     cl,
			virtClient: virtClient,
		},
	}
}

type USBDeviceMigrationHandler struct {
	usbDeviceHandlerBase
}

func (h *USBDeviceMigrationHandler) Name() string {
	return nameUSBDeviceMigrationHandler
}

func (h *USBDeviceMigrationHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	migratingCond, exists := conditions.GetCondition(vmcondition.TypeMigratable, changed.Status.Conditions)
	if !exists {
		return reconcile.Result{}, nil
	}

	if migratingCond.Reason != vmcondition.ReasonUSBShouldBeMigrating.String() {
		return reconcile.Result{}, nil
	}

	hasPendingMigration, err := h.hasPendingMigrationOp(ctx, s)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !hasPendingMigration {
		return reconcile.Result{}, nil
	}

	for i := 0; i < len(changed.Status.USBDevices); i++ {
		ref := &changed.Status.USBDevices[i]
		err := h.detachUSBDevice(ctx, vm, ref.Name)
		if err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to unplug USB device for migration %s: %w", ref.Name, err)
		}

		ref.Attached = false
		ref.Address = nil
		ref.Hotplugged = false
	}

	return reconcile.Result{}, nil
}
