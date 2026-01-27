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
	"strings"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/hash"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameReadyHandler = "ReadyHandler"
)

func NewReadyHandler(recorder eventrecord.EventRecorderLogger) *ReadyHandler {
	return &ReadyHandler{
		recorder: recorder,
	}
}

type ReadyHandler struct {
	recorder eventrecord.EventRecorderLogger
}

func (h *ReadyHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	nodeUSBDevice := s.NodeUSBDevice()

	if nodeUSBDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := nodeUSBDevice.Current()
	changed := nodeUSBDevice.Changed()

	// Check if device exists in ResourceSlice
	resourceSlices, err := s.ResourceSlices(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get resource slices: %w", err)
	}

	deviceFound := h.findDeviceInSlices(resourceSlices, current.Status.Attributes.Hash, current.Status.NodeName)

	var reason nodeusbdevicecondition.ReadyReason
	var message string
	var status metav1.ConditionStatus

	if !deviceFound {
		// Device not found - mark as NotFound
		reason = nodeusbdevicecondition.NotFound
		message = "Device is absent on the host"
		status = metav1.ConditionFalse
	} else {
		// Device found - check if it's ready
		// For now, if device exists in ResourceSlice, we consider it ready
		reason = nodeusbdevicecondition.Ready
		message = "Device is ready to use"
		status = metav1.ConditionTrue
	}

	cb := conditions.NewConditionBuilder(nodeusbdevicecondition.ReadyType).
		Generation(current.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *ReadyHandler) findDeviceInSlices(slices []resourcev1beta1.ResourceSlice, searchedHash, nodeName string) bool {
	for _, slice := range slices {
		if slice.Spec.Pool.Name != nodeName {
			continue
		}

		for _, device := range slice.Spec.Devices {
			if !strings.HasPrefix(device.Name, "virtualization-dra-") {
				continue
			}

			// Calculate hash for this device and compare
			deviceHash := hash.CalculateHashFromDevice(device, nodeName)
			if deviceHash == searchedHash {
				return true
			}
		}
	}

	return false
}

func (h *ReadyHandler) Name() string {
	return nameReadyHandler
}
