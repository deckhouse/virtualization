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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameDeletionHandler = "DeletionHandler"
)

func NewDeletionHandler(client client.Client) *DeletionHandler {
	return &DeletionHandler{
		client: client,
	}
}

type DeletionHandler struct {
	client client.Client
}

func (h *DeletionHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	nodeUSBDevice := s.NodeUSBDevice()

	if nodeUSBDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := nodeUSBDevice.Current()
	changed := nodeUSBDevice.Changed()

	switch {
	case current.GetDeletionTimestamp().IsZero():
		if shouldAutoDeleteNodeUSBDevice(current) {
			if err := h.cleanupOwnedUSBDevices(ctx, current); err != nil {
				return reconcile.Result{}, err
			}
			if err := h.client.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
				return reconcile.Result{}, fmt.Errorf("failed to delete NodeUSBDevice: %w", err)
			}
			return reconcile.Result{}, reconciler.ErrStopHandlerChain
		}

		controllerutil.AddFinalizer(changed, v1alpha2.FinalizerNodeUSBDeviceCleanup)
		return reconcile.Result{}, nil

	default:
		if err := h.cleanupOwnedUSBDevices(ctx, current); err != nil {
			return reconcile.Result{}, err
		}
		controllerutil.RemoveFinalizer(changed, v1alpha2.FinalizerNodeUSBDeviceCleanup)
	}

	return reconcile.Result{}, nil
}

func (h *DeletionHandler) cleanupOwnedUSBDevices(ctx context.Context, owner *v1alpha2.NodeUSBDevice) error {
	var usbDeviceList v1alpha2.USBDeviceList
	if err := h.client.List(ctx, &usbDeviceList); err != nil {
		return fmt.Errorf("failed to list USBDevices: %w", err)
	}

	for i := range usbDeviceList.Items {
		usbDevice := &usbDeviceList.Items[i]
		if !metav1.IsControlledBy(usbDevice, owner) {
			continue
		}

		if err := h.client.Delete(ctx, usbDevice); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete USBDevice %s/%s: %w", usbDevice.Namespace, usbDevice.Name, err)
		}
	}

	return nil
}

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}

func shouldAutoDeleteNodeUSBDevice(nodeUSBDevice *v1alpha2.NodeUSBDevice) bool {
	if nodeUSBDevice == nil || nodeUSBDevice.GetDeletionTimestamp() != nil {
		return false
	}

	if nodeUSBDevice.Spec.AssignedNamespace != "" {
		return false
	}

	readyCondition := meta.FindStatusCondition(nodeUSBDevice.Status.Conditions, string(nodeusbdevicecondition.ReadyType))
	if readyCondition == nil {
		return false
	}

	return readyCondition.Status == metav1.ConditionFalse && readyCondition.Reason == string(nodeusbdevicecondition.NotFound)
}
