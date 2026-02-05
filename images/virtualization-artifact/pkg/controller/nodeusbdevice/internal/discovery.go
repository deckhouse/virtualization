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
	resourceSlices, err := s.ResourceSlices(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get resource slices: %w", err)
	}

	var existingDevices v1alpha2.NodeUSBDeviceList
	if err := h.client.List(ctx, &existingDevices); err != nil {
		log.Error("failed to discover and create NodeUSBDevice", log.Err(fmt.Errorf("failed to list existing NodeUSBDevices: %w", err)))
	} else {
		existingDeviceNames := make(map[string]bool)
		for _, device := range existingDevices.Items {
			if device.Status.Attributes.Name != "" {
				existingDeviceNames[device.Status.Attributes.Name] = true
			}
		}

		var discoverErr error
		for _, slice := range resourceSlices {
			if discoverErr != nil {
				break
			}
			for _, device := range slice.Spec.Devices {
				if !IsUSBDevice(device) {
					continue
				}
				if existingDeviceNames[device.Name] {
					continue
				}
				attributes := ConvertDeviceToAttributes(device, slice.Spec.Pool.Name)
				if err := h.createNodeUSBDevice(ctx, attributes); err != nil {
					discoverErr = err
					break
				}
			}
		}
		if discoverErr != nil {
			log.Error("failed to discover and create NodeUSBDevice", log.Err(discoverErr))
		}
	}

	return reconcile.Result{}, nil
}

func (h *DiscoveryHandler) createNodeUSBDevice(ctx context.Context, attributes v1alpha2.NodeUSBDeviceAttributes) error {
	name := h.sanitizeName(attributes.Name)

	existing := &v1alpha2.NodeUSBDevice{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check if NodeUSBDevice exists: %w", err)
	}

	nodeUSBDevice := &v1alpha2.NodeUSBDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.NodeUSBDeviceSpec{
			AssignedNamespace: "",
		},
	}

	if err := h.client.Create(ctx, nodeUSBDevice); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create NodeUSBDevice: %w", err)
	}

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

func (h *DiscoveryHandler) sanitizeName(deviceName string) string {
	return strings.ToLower(strings.ReplaceAll(deviceName, ".", "-"))
}
