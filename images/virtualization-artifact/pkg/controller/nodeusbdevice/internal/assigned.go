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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
)

const (
	nameAssignedHandler = "AssignedHandler"
)

func NewAssignedHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *AssignedHandler {
	return &AssignedHandler{
		client:   client,
		recorder: recorder,
	}
}

type AssignedHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *AssignedHandler) Name() string {
	return nameAssignedHandler
}

func (h *AssignedHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	nodeUSBDevice := s.NodeUSBDevice()

	if nodeUSBDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := nodeUSBDevice.Current()
	changed := nodeUSBDevice.Changed()

	assignedNamespace := current.Spec.AssignedNamespace

	// Check previous assignedNamespace if it changed
	// Try to find previous USBDevice to delete it if namespace changed
	var usbDeviceList v1alpha2.USBDeviceList
	if err := h.client.List(ctx, &usbDeviceList); err == nil {
		for _, usbDevice := range usbDeviceList.Items {
			if usbDevice.Name == current.Name && usbDevice.Namespace != assignedNamespace {
				// Delete USBDevice from previous namespace
				if err := h.deleteUSBDevice(ctx, usbDevice.Namespace, usbDevice.Name); err != nil {
					return reconcile.Result{}, fmt.Errorf("failed to delete USBDevice from previous namespace: %w", err)
				}
				break
			}
		}
	}

	// Update Assigned condition
	var reason nodeusbdevicecondition.AssignedReason
	var message string
	var status metav1.ConditionStatus

	if assignedNamespace != "" {
		// Check if namespace exists
		var namespace corev1.Namespace
		err := h.client.Get(ctx, types.NamespacedName{Name: assignedNamespace}, &namespace)
		if err != nil {
			if errors.IsNotFound(err) {
				// Namespace doesn't exist - mark as Available
				reason = nodeusbdevicecondition.Available
				message = fmt.Sprintf("Namespace %s does not exist", assignedNamespace)
				status = metav1.ConditionFalse
			} else {
				// Error checking namespace - return error to retry
				return reconcile.Result{}, fmt.Errorf("failed to check namespace %s: %w", assignedNamespace, err)
			}
		} else {
			// Namespace exists - create or update USBDevice
			usbDevice, err := h.ensureUSBDevice(ctx, current, assignedNamespace)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to ensure USBDevice: %w", err)
			}

			if usbDevice != nil {
				reason = nodeusbdevicecondition.Assigned
				message = fmt.Sprintf("Namespace %s is assigned for the device, USBDevice created", assignedNamespace)
				status = metav1.ConditionTrue
			} else {
				reason = nodeusbdevicecondition.InProgress
				message = fmt.Sprintf("Creating USBDevice in namespace %s", assignedNamespace)
				status = metav1.ConditionFalse
			}
		}
	} else {
		// No namespace assigned - delete USBDevice if it exists
		var usbDeviceList v1alpha2.USBDeviceList
		if err := h.client.List(ctx, &usbDeviceList); err == nil {
			for _, usbDevice := range usbDeviceList.Items {
				if usbDevice.Name == current.Name {
					if err := h.deleteUSBDevice(ctx, usbDevice.Namespace, usbDevice.Name); err != nil {
						return reconcile.Result{}, fmt.Errorf("failed to delete USBDevice: %w", err)
					}
				}
			}
		}

		reason = nodeusbdevicecondition.Available
		message = "No namespace is assigned for the device"
		status = metav1.ConditionFalse
	}

	cb := conditions.NewConditionBuilder(nodeusbdevicecondition.AssignedType).
		Generation(current.GetGeneration()).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *AssignedHandler) ensureUSBDevice(ctx context.Context, nodeUSBDevice *v1alpha2.NodeUSBDevice, namespace string) (*v1alpha2.USBDevice, error) {
	usbDevice := &v1alpha2.USBDevice{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      nodeUSBDevice.Name,
	}

	err := h.client.Get(ctx, key, usbDevice)
	if err == nil {
		// USBDevice exists - check if update is needed
		needsUpdate := !reflect.DeepEqual(usbDevice.Status.Attributes, nodeUSBDevice.Status.Attributes) ||
			usbDevice.Status.NodeName != nodeUSBDevice.Status.NodeName

		if needsUpdate {
			usbDevice.Status.Attributes = nodeUSBDevice.Status.Attributes
			usbDevice.Status.NodeName = nodeUSBDevice.Status.NodeName
			if err := h.client.Status().Update(ctx, usbDevice); err != nil {
				return nil, fmt.Errorf("failed to update USBDevice status: %w", err)
			}
		}
		return usbDevice, nil
	}

	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get USBDevice: %w", err)
	}

	// USBDevice doesn't exist - create it
	// Create USBDevice without status (status is a subresource)
	usbDevice = &v1alpha2.USBDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeUSBDevice.Name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: nodeUSBDevice.APIVersion,
					Kind:       nodeUSBDevice.Kind,
					Name:       nodeUSBDevice.Name,
					UID:        nodeUSBDevice.UID,
					Controller: ptr.To(true),
				},
			},
		},
	}

	if err := h.client.Create(ctx, usbDevice); err != nil {
		return nil, fmt.Errorf("failed to create USBDevice: %w", err)
	}

	// Update status separately (status is a subresource)
	usbDevice.Status = v1alpha2.USBDeviceStatus{
		Attributes: nodeUSBDevice.Status.Attributes,
		NodeName:   nodeUSBDevice.Status.NodeName,
	}

	if err := h.client.Status().Update(ctx, usbDevice); err != nil {
		return nil, fmt.Errorf("failed to update USBDevice status: %w", err)
	}

	return usbDevice, nil
}

func (h *AssignedHandler) deleteUSBDevice(ctx context.Context, namespace, name string) error {
	usbDevice := &v1alpha2.USBDevice{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	err := h.client.Get(ctx, key, usbDevice)
	if err != nil {
		if errors.IsNotFound(err) {
			// USBDevice doesn't exist - nothing to delete
			return nil
		}
		return fmt.Errorf("failed to get USBDevice: %w", err)
	}

	if err := h.client.Delete(ctx, usbDevice); err != nil {
		return fmt.Errorf("failed to delete USBDevice: %w", err)
	}

	return nil
}
