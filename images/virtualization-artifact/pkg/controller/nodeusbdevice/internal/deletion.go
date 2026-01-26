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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	nameDeletionHandler = "DeletionHandler"
)

func NewDeletionHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *DeletionHandler {
	return &DeletionHandler{
		client:   client,
		recorder: recorder,
	}
}

type DeletionHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *DeletionHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	nodeUSBDevice := s.NodeUSBDevice()

	if nodeUSBDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := nodeUSBDevice.Current()
	changed := nodeUSBDevice.Changed()

	// Add finalizer if not deleting
	if current.GetDeletionTimestamp().IsZero() {
		controllerutil.AddFinalizer(changed, v1alpha2.FinalizerNodeUSBDeviceCleanup)
		return reconcile.Result{}, nil
	}

	// TODO: When USBDevice resource is defined, delete it from namespace here
	// Resource is being deleted - clean up USBDevice in namespace
	// if current.Spec.AssignedNamespace != "" {
	// 	if err := h.deleteUSBDevice(ctx, current.Spec.AssignedNamespace, current); err != nil {
	// 		return reconcile.Result{}, err
	// 	}
	// }

	// Remove finalizer
	controllerutil.RemoveFinalizer(changed, v1alpha2.FinalizerNodeUSBDeviceCleanup)

	return reconcile.Result{}, nil
}

// TODO: Implement USBDevice deletion when USBDevice resource is defined

func (h *DeletionHandler) Name() string {
	return nameDeletionHandler
}
