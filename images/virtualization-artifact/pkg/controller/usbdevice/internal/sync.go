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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
)

const (
	nameSyncHandler = "SyncHandler"
)

func NewSyncHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *SyncHandler {
	return &SyncHandler{
		client:   client,
		recorder: recorder,
	}
}

type SyncHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *SyncHandler) Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error) {
	usbDevice := s.USBDevice()

	if usbDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	changed := usbDevice.Changed()

	// Get corresponding NodeUSBDevice
	nodeUSBDevice, err := s.NodeUSBDevice(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if nodeUSBDevice == nil {
		// NodeUSBDevice not found - nothing to sync
		return reconcile.Result{}, nil
	}

	// Sync attributes from NodeUSBDevice
	changed.Status.Attributes = nodeUSBDevice.Status.Attributes
	changed.Status.NodeName = nodeUSBDevice.Status.NodeName

	return reconcile.Result{}, nil
}

func (h *SyncHandler) Name() string {
	return nameSyncHandler
}
