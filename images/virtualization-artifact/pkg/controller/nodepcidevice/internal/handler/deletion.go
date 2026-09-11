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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodepcidevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodepcidevicecondition"
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

func (h *DeletionHandler) Handle(ctx context.Context, s state.NodePCIDeviceState) (reconcile.Result, error) {
	nodePCIDevice := s.NodePCIDevice()

	if nodePCIDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := nodePCIDevice.Current()
	changed := nodePCIDevice.Changed()

	switch {
	case current.GetDeletionTimestamp().IsZero():
		if !controllerutil.ContainsFinalizer(current, v1alpha2.FinalizerNodePCIDeviceCleanup) {
			controllerutil.AddFinalizer(changed, v1alpha2.FinalizerNodePCIDeviceCleanup)
			return reconcile.Result{}, nil
		}

		if shouldAutoDeleteNodePCIDevice(current) {
			if err := h.client.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
				return reconcile.Result{}, fmt.Errorf("failed to delete NodePCIDevice: %w", err)
			}
			return reconcile.Result{}, reconciler.ErrStopHandlerChain
		}

		return reconcile.Result{}, nil

	default:
		remaining, err := h.cleanupOwnedPCIDevices(ctx, current)
		if err != nil {
			return reconcile.Result{}, err
		}
		if remaining > 0 {
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}
		controllerutil.RemoveFinalizer(changed, v1alpha2.FinalizerNodePCIDeviceCleanup)
	}

	return reconcile.Result{}, nil
}

// cleanupOwnedPCIDevices deletes the PCIDevices owned by the NodePCIDevice and
// returns how many of them still exist: a PCIDevice referenced by a
// VirtualMachine keeps its finalizer until the reference is gone.
func (h *DeletionHandler) cleanupOwnedPCIDevices(ctx context.Context, owner *v1alpha2.NodePCIDevice) (int, error) {
	var pciDeviceList v1alpha2.PCIDeviceList
	if err := h.client.List(ctx, &pciDeviceList); err != nil {
		return 0, fmt.Errorf("failed to list PCIDevices: %w", err)
	}

	remaining := 0
	for i := range pciDeviceList.Items {
		pciDevice := &pciDeviceList.Items[i]
		if !metav1.IsControlledBy(pciDevice, owner) {
			continue
		}
		remaining++
		if !pciDevice.GetDeletionTimestamp().IsZero() {
			continue
		}
		if err := h.client.Delete(ctx, pciDevice); err != nil {
			if apierrors.IsNotFound(err) {
				remaining--
				continue
			}
			return 0, fmt.Errorf("failed to delete PCIDevice %s/%s: %w", pciDevice.Namespace, pciDevice.Name, err)
		}
		if len(pciDevice.GetFinalizers()) == 0 {
			remaining--
		}
	}

	return remaining, nil
}

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}

func shouldAutoDeleteNodePCIDevice(nodePCIDevice *v1alpha2.NodePCIDevice) bool {
	if nodePCIDevice == nil || nodePCIDevice.GetDeletionTimestamp() != nil {
		return false
	}

	if nodePCIDevice.Spec.AssignedNamespace != "" {
		return false
	}

	readyCondition := meta.FindStatusCondition(nodePCIDevice.Status.Conditions, string(nodepcidevicecondition.ReadyType))
	if readyCondition == nil {
		return false
	}

	return readyCondition.Status == metav1.ConditionFalse && readyCondition.Reason == string(nodepcidevicecondition.NotFound)
}
