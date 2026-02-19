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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	nameAssignedHandler = "AssignedHandler"
)

func NewAssignedHandler(client client.Client) *AssignedHandler {
	return &AssignedHandler{
		client: client,
	}
}

type AssignedHandler struct {
	client client.Client
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

	if !current.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	assignedNamespace := current.Spec.AssignedNamespace
	deviceAbsentOnHost := isDeviceAbsentOnHost(changed.Status.Conditions)

	switch {
	case deviceAbsentOnHost:
		if err := h.removeOrphanedUSBDevices(ctx, current.Name, assignedNamespace, true); err != nil {
			return reconcile.Result{}, err
		}
		setAssignedInProgressCondition(current, &changed.Status.Conditions, "Device is absent on the host, USBDevice is removed.")

	case assignedNamespace == "":
		if err := h.removeOrphanedUSBDevices(ctx, current.Name, assignedNamespace, false); err != nil {
			return reconcile.Result{}, err
		}
		setAssignedAvailableCondition(current, &changed.Status.Conditions, "No namespace is assigned for the device.")

	default:
		if err := h.removeOrphanedUSBDevices(ctx, current.Name, assignedNamespace, false); err != nil {
			return reconcile.Result{}, err
		}

		exists, err := h.namespaceExists(ctx, assignedNamespace)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to check namespace %s: %w", assignedNamespace, err)
		}
		if !exists {
			setAssignedAvailableCondition(current, &changed.Status.Conditions, fmt.Sprintf("Namespace %s does not exist.", assignedNamespace))
			return reconcile.Result{}, nil
		}

		if _, err := h.ensureUSBDevice(ctx, current, assignedNamespace); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to ensure USBDevice: %w", err)
		}

		setAssignedReadyCondition(current, &changed.Status.Conditions, assignedNamespace)
	}

	return reconcile.Result{}, nil
}

func (h *AssignedHandler) namespaceExists(ctx context.Context, name string) (bool, error) {
	var namespace corev1.Namespace
	err := h.client.Get(ctx, types.NamespacedName{Name: name}, &namespace)
	if err == nil {
		return true, nil
	}
	if errors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

func (h *AssignedHandler) removeOrphanedUSBDevices(ctx context.Context, deviceName, assignedNamespace string, deviceAbsentOnHost bool) error {
	var usbDeviceList v1alpha2.USBDeviceList
	if err := h.client.List(ctx, &usbDeviceList, client.MatchingFields{indexer.IndexFieldUSBDeviceByName: deviceName}); err != nil {
		return fmt.Errorf("failed to list USBDevices: %w", err)
	}

	for _, usbDevice := range usbDeviceList.Items {
		if deviceAbsentOnHost || assignedNamespace == "" || usbDevice.Namespace != assignedNamespace {
			if err := h.deleteUSBDevice(ctx, usbDevice.Namespace, usbDevice.Name); err != nil {
				return fmt.Errorf("failed to delete USBDevice %s/%s: %w", usbDevice.Namespace, usbDevice.Name, err)
			}
		}
	}

	return nil
}

func (h *AssignedHandler) ensureUSBDevice(ctx context.Context, nodeUSBDevice *v1alpha2.NodeUSBDevice, namespace string) (*v1alpha2.USBDevice, error) {
	usbDevice := &v1alpha2.USBDevice{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      nodeUSBDevice.Name,
	}

	err := h.client.Get(ctx, key, usbDevice)
	if err == nil {
		if !equality.Semantic.DeepEqual(usbDevice.Status.Attributes, nodeUSBDevice.Status.Attributes) || usbDevice.Status.NodeName != nodeUSBDevice.Status.NodeName {
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
	usbDevice = &v1alpha2.USBDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeUSBDevice.Name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.NodeUSBDeviceKind,
					Name:       nodeUSBDevice.Name,
					UID:        nodeUSBDevice.UID,
					Controller: ptr.To(true),
				},
			},
		},
		Status: v1alpha2.USBDeviceStatus{
			Attributes: nodeUSBDevice.Status.Attributes,
			NodeName:   nodeUSBDevice.Status.NodeName,
		},
	}

	if err := h.client.Create(ctx, usbDevice); err != nil {
		if errors.IsAlreadyExists(err) {
			if err := h.client.Get(ctx, key, usbDevice); err != nil {
				return nil, fmt.Errorf("failed to get existing USBDevice: %w", err)
			}
			if !equality.Semantic.DeepEqual(usbDevice.Status.Attributes, nodeUSBDevice.Status.Attributes) || usbDevice.Status.NodeName != nodeUSBDevice.Status.NodeName {
				usbDevice.Status.Attributes = nodeUSBDevice.Status.Attributes
				usbDevice.Status.NodeName = nodeUSBDevice.Status.NodeName
				if err := h.client.Status().Update(ctx, usbDevice); err != nil {
					return nil, fmt.Errorf("failed to update USBDevice status: %w", err)
				}
			}
			return usbDevice, nil
		}
		return nil, fmt.Errorf("failed to create USBDevice: %w", err)
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
