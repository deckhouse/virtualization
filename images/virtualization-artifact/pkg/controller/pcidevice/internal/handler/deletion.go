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

package handler

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/pcidevice/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	nameDeletionHandler = "DeletionHandler"
)

func NewDeletionHandler() *DeletionHandler {
	return &DeletionHandler{}
}

// DeletionHandler keeps the cleanup finalizer on the PCIDevice while some
// VirtualMachine references it. PCI devices are cold-plugged, so there is no
// hot detach to perform: the device is released only when the user removes it
// from the virtual machine spec.
type DeletionHandler struct{}

func (h *DeletionHandler) Handle(ctx context.Context, s state.PCIDeviceState) (reconcile.Result, error) {
	pciDevice := s.PCIDevice()

	if pciDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := pciDevice.Current()
	changed := pciDevice.Changed()

	if current.GetDeletionTimestamp().IsZero() {
		controllerutil.AddFinalizer(changed, v1alpha2.FinalizerPCIDeviceCleanup)
		return reconcile.Result{}, nil
	}

	vms, err := s.VirtualMachinesReferencingDevice(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to find VirtualMachines referencing PCIDevice: %w", err)
	}

	if len(vms) > 0 {
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	controllerutil.RemoveFinalizer(changed, v1alpha2.FinalizerPCIDeviceCleanup)

	return reconcile.Result{}, nil
}

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}
