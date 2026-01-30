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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameDiscoveryHandler = "DiscoveryHandler"
)

func NewDiscoveryHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *DiscoveryHandler {
	return &DiscoveryHandler{
		client:   client,
		recorder: recorder,
	}
}

type DiscoveryHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *DiscoveryHandler) Name() string {
	return nameDiscoveryHandler
}

func (h *DiscoveryHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	// Get ResourceSlices
	resourceSlices, err := s.ResourceSlices(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get resource slices: %w", err)
	}

	// Check for new devices in ResourceSlice and create NodeUSBDevice if needed
	// This ensures we discover new devices even if reconcile was triggered for other reasons
	if err := h.discoverAndCreate(ctx, s, resourceSlices); err != nil {
		// Log error but don't fail reconciliation
		// This is a best-effort discovery mechanism
		log.Error("failed to discover and create NodeUSBDevice", log.Err(err))
	}

	return reconcile.Result{}, nil
}

func (h *DiscoveryHandler) discoverAndCreate(ctx context.Context, s state.NodeUSBDeviceState, resourceSlices []resourcev1beta1.ResourceSlice) error {
	// Check if current device exists - if it does, we only need to check for new devices
	// This avoids unnecessary List when reconciling existing devices
	currentDevice := s.NodeUSBDevice()
	hasCurrentDevice := !currentDevice.IsEmpty()

	// Collect all device names from ResourceSlices (name is unique, guaranteed by DRA driver)
	deviceNamesInSlices := make(map[string]bool)
	for _, slice := range resourceSlices {
		for _, device := range slice.Spec.Devices {
			if !IsUSBDevice(device) {
				continue
			}
			deviceNamesInSlices[device.Name] = true
		}
	}

	// If we have a current device and its name is in slices, we can skip List
	if hasCurrentDevice {
		current := currentDevice.Current()
		if current.Status.Attributes.Name != "" && deviceNamesInSlices[current.Status.Attributes.Name] {
			var existingDevices v1alpha2.NodeUSBDeviceList
			if err := h.client.List(ctx, &existingDevices); err != nil {
				return fmt.Errorf("failed to list existing NodeUSBDevices: %w", err)
			}

			existingDeviceNames := make(map[string]bool)
			for _, device := range existingDevices.Items {
				if device.Status.Attributes.Name != "" {
					existingDeviceNames[device.Status.Attributes.Name] = true
				}
			}

			for _, slice := range resourceSlices {
				for _, device := range slice.Spec.Devices {
					if !IsUSBDevice(device) {
						continue
					}
					if existingDeviceNames[device.Name] {
						continue
					}
					attributes := ConvertDeviceToAttributes(device, slice.Spec.Pool.Name)
					if err := h.createNodeUSBDevice(ctx, attributes); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}

	// No current device or it's not in slices - need full List
	var existingDevices v1alpha2.NodeUSBDeviceList
	if err := h.client.List(ctx, &existingDevices); err != nil {
		return fmt.Errorf("failed to list existing NodeUSBDevices: %w", err)
	}

	existingDeviceNames := make(map[string]bool)
	for _, device := range existingDevices.Items {
		if device.Status.Attributes.Name != "" {
			existingDeviceNames[device.Status.Attributes.Name] = true
		}
	}

	// Create NodeUSBDevice for each USB device in ResourceSlices
	for _, slice := range resourceSlices {
		for _, device := range slice.Spec.Devices {
			if !IsUSBDevice(device) {
				continue
			}
			if existingDeviceNames[device.Name] {
				continue
			}
			attributes := ConvertDeviceToAttributes(device, slice.Spec.Pool.Name)
			if err := h.createNodeUSBDevice(ctx, attributes); err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *DiscoveryHandler) createNodeUSBDevice(ctx context.Context, attributes v1alpha2.NodeUSBDeviceAttributes) error {
	name := h.sanitizeName(attributes.Name)

	// Check if device already exists
	existing := &v1alpha2.NodeUSBDevice{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name}, existing)
	if err == nil {
		// Device already exists, skip creation
		return nil
	}
	if !apierrors.IsNotFound(err) {
		// Unexpected error
		return fmt.Errorf("failed to check if NodeUSBDevice exists: %w", err)
	}

	// Create NodeUSBDevice without status (status is a subresource)
	nodeUSBDevice := &v1alpha2.NodeUSBDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.NodeUSBDeviceSpec{
			AssignedNamespace: "",
		},
	}

	if err := h.client.Create(ctx, nodeUSBDevice); err != nil {
		// If device was created by another process between check and create, ignore the error
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create NodeUSBDevice: %w", err)
	}

	// Update status separately (status is a subresource)
	nodeUSBDevice.Status = v1alpha2.NodeUSBDeviceStatus{
		Attributes: attributes,
		NodeName:   attributes.NodeName,
		Conditions: []metav1.Condition{
			{
				Type:               string(nodeusbdevicecondition.ReadyType),
				Status:             metav1.ConditionTrue,
				Reason:             string(nodeusbdevicecondition.Ready),
				Message:            "Device is ready to use",
				LastTransitionTime: metav1.Now(),
			},
			{
				Type:               string(nodeusbdevicecondition.AssignedType),
				Status:             metav1.ConditionFalse,
				Reason:             string(nodeusbdevicecondition.Available),
				Message:            "No namespace is assigned for the device",
				LastTransitionTime: metav1.Now(),
			},
		},
	}

	if err := h.client.Status().Update(ctx, nodeUSBDevice); err != nil {
		return fmt.Errorf("failed to update NodeUSBDevice status: %w", err)
	}

	return nil
}

// sanitizeName converts ResourceSlice device name to a valid Kubernetes resource name.
// Device name is unique and guaranteed by the DRA driver.
func (h *DiscoveryHandler) sanitizeName(deviceName string) string {
	return strings.ToLower(strings.ReplaceAll(deviceName, ".", "-"))
}
