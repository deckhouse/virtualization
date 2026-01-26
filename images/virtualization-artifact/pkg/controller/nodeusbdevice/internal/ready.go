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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameReadyHandler = "ReadyHandler"
)

func NewReadyHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *ReadyHandler {
	return &ReadyHandler{
		client:   client,
		recorder: recorder,
	}
}

type ReadyHandler struct {
	client   client.Client
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
	resourceSlices, err := h.getResourceSlices(ctx)
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

func (h *ReadyHandler) getResourceSlices(ctx context.Context) ([]resourcev1beta1.ResourceSlice, error) {
	var slices resourcev1beta1.ResourceSliceList
	if err := h.client.List(ctx, &slices, client.MatchingLabels{}); err != nil {
		return nil, err
	}

	result := make([]resourcev1beta1.ResourceSlice, 0)
	for _, slice := range slices.Items {
		if slice.Spec.Driver == draDriverName {
			result = append(result, slice)
		}
	}

	return result, nil
}

func (h *ReadyHandler) findDeviceInSlices(slices []resourcev1beta1.ResourceSlice, hash, nodeName string) bool {
	for _, slice := range slices {
		if slice.Spec.Pool.Name != nodeName {
			continue
		}

		for _, device := range slice.Spec.Devices {
			if !strings.HasPrefix(device.Name, "usb-") {
				continue
			}

			// Calculate hash for this device and compare
			deviceHash := h.calculateDeviceHash(device, nodeName)
			if deviceHash == hash {
				return true
			}
		}
	}

	return false
}

func (h *ReadyHandler) calculateDeviceHash(device resourcev1beta1.Device, nodeName string) string {
	// Extract attributes and calculate hash similar to discovery handler
	var vendorID, productID, bus, deviceNumber, serial, devicePath string

	if device.Basic != nil {
		for key, attr := range device.Basic.Attributes {
			switch string(key) {
			case "vendorID":
				if attr.StringValue != nil {
					vendorID = *attr.StringValue
				}
			case "productID":
				if attr.StringValue != nil {
					productID = *attr.StringValue
				}
			case "bus":
				if attr.StringValue != nil {
					bus = *attr.StringValue
				}
			case "deviceNumber":
				if attr.StringValue != nil {
					deviceNumber = *attr.StringValue
				}
			case "serial":
				if attr.StringValue != nil {
					serial = *attr.StringValue
				}
			case "devicePath":
				if attr.StringValue != nil {
					devicePath = *attr.StringValue
				}
			}
		}
	}

	hashInput := fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s",
		nodeName, vendorID, productID, bus, deviceNumber, serial, devicePath)

	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:])[:16]
}

func (h *ReadyHandler) Name() string {
	return nameReadyHandler
}
