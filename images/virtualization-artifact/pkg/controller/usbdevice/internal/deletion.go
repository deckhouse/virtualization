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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

const (
	nameDeletionHandler = "DeletionHandler"
)

func NewDeletionHandler(client client.Client, virtClient versioned.Interface, recorder eventrecord.EventRecorderLogger) *DeletionHandler {
	return &DeletionHandler{
		client:     client,
		virtClient: virtClient,
		recorder:   recorder,
	}
}

type DeletionHandler struct {
	client     client.Client
	virtClient versioned.Interface
	recorder   eventrecord.EventRecorderLogger
}

func (h *DeletionHandler) Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error) {
	usbDevice := s.USBDevice()

	if usbDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := usbDevice.Current()
	changed := usbDevice.Changed()

	if current.GetDeletionTimestamp().IsZero() {
		controllerutil.AddFinalizer(changed, v1alpha2.FinalizerUSBDeviceCleanup)
		return reconcile.Result{}, nil
	}

	vms, err := s.VirtualMachinesUsingDevice(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to find VirtualMachines using USBDevice: %w", err)
	}

	if len(vms) > 0 {
		h.recorder.Eventf(changed, "Normal", "Deletion", "Device is attached to VM(s), performing hot unplug")

		for _, vm := range vms {
			err := h.virtClient.VirtualizationV1alpha2().VirtualMachines(vm.Namespace).RemoveResourceClaim(ctx, vm.Name, subv1alpha2.VirtualMachineRemoveResourceClaim{Name: current.Name})
			if err == nil {
				h.recorder.Eventf(changed, "Normal", "Deletion", "Removed ResourceClaim from VM %s/%s", vm.Namespace, vm.Name)
				continue
			}

			if apierrors.IsNotFound(err) {
				continue
			}

			h.recorder.Eventf(changed, "Warning", "Deletion", "Failed to remove ResourceClaim from VM %s/%s: %v", vm.Namespace, vm.Name, err)
			return reconcile.Result{Requeue: true}, fmt.Errorf("failed to remove ResourceClaim from VM %s/%s: %w", vm.Namespace, vm.Name, err)
		}

		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	controllerutil.RemoveFinalizer(changed, v1alpha2.FinalizerUSBDeviceCleanup)

	return reconcile.Result{}, nil
}

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}
