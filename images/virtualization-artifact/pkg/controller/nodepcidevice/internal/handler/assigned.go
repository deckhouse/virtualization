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
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodepcidevice/internal/state"
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

var errPCIDeviceTerminating = errors.New("PCIDevice is being deleted")

// pciDeviceAbsenceGracePeriod is how long a device may be missing from the
// ResourceSlices before its PCIDevice is removed. A kubelet or plugin restart
// hides the device for up to a minute, a node reboot for several minutes and
// the driver rescans every five minutes; only a device that is still gone
// after that is treated as physically removed.
const pciDeviceAbsenceGracePeriod = 10 * time.Minute

type AssignedHandler struct {
	client client.Client
}

func (h *AssignedHandler) Name() string {
	return nameAssignedHandler
}

func (h *AssignedHandler) Handle(ctx context.Context, s state.NodePCIDeviceState) (reconcile.Result, error) {
	nodePCIDevice := s.NodePCIDevice()

	if nodePCIDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := nodePCIDevice.Current()
	changed := nodePCIDevice.Changed()

	if !current.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	assignedNamespace := current.Spec.AssignedNamespace

	if assignedNamespace == "" {
		if err := h.removeOrphanedPCIDevices(ctx, current.Name, assignedNamespace); err != nil {
			return reconcile.Result{}, err
		}
		setAssignedAvailableCondition(current, &changed.Status.Conditions, "No namespace is assigned for the device.")
		return reconcile.Result{}, nil
	}

	if err := h.removeOrphanedPCIDevices(ctx, current.Name, assignedNamespace); err != nil {
		return reconcile.Result{}, err
	}

	if absentFor, absent := deviceAbsenceDuration(changed.Status.Conditions); absent {
		if absentFor < pciDeviceAbsenceGracePeriod {
			return h.ensureAssignedPCIDevice(ctx, current, changed, assignedNamespace, pciDeviceAbsenceGracePeriod-absentFor)
		}
		if err := h.deletePCIDevice(ctx, assignedNamespace, current.Name); err != nil {
			return reconcile.Result{}, err
		}
		setAssignedInProgressCondition(current, &changed.Status.Conditions, fmt.Sprintf("Device has been absent on the host for more than %s, the PCIDevice is removed.", pciDeviceAbsenceGracePeriod))
		return reconcile.Result{}, nil
	}

	return h.ensureAssignedPCIDevice(ctx, current, changed, assignedNamespace, 0)
}

// ensureAssignedPCIDevice keeps the PCIDevice in the assigned namespace in
// sync; requeueAfter is set while the device is absent so that the handler
// comes back to remove the PCIDevice once the grace period is over.
func (h *AssignedHandler) ensureAssignedPCIDevice(ctx context.Context, current, changed *v1alpha2.NodePCIDevice, assignedNamespace string, requeueAfter time.Duration) (reconcile.Result, error) {
	exists, err := h.namespaceExists(ctx, assignedNamespace)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to check namespace %s: %w", assignedNamespace, err)
	}
	if !exists {
		setAssignedAvailableCondition(current, &changed.Status.Conditions, fmt.Sprintf("Namespace %s does not exist.", assignedNamespace))
		return reconcile.Result{}, nil
	}

	if _, err := h.ensurePCIDevice(ctx, current, assignedNamespace); err != nil {
		if errors.Is(err, errPCIDeviceTerminating) {
			setAssignedInProgressCondition(current, &changed.Status.Conditions, fmt.Sprintf("Waiting for the previous PCIDevice in namespace %q to be deleted.", assignedNamespace))
			return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to ensure PCIDevice: %w", err)
	}

	setAssignedReadyCondition(current, &changed.Status.Conditions, assignedNamespace)

	return reconcile.Result{RequeueAfter: requeueAfter}, nil
}

func (h *AssignedHandler) namespaceExists(ctx context.Context, name string) (bool, error) {
	var namespace corev1.Namespace
	err := h.client.Get(ctx, types.NamespacedName{Name: name}, &namespace)
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

func (h *AssignedHandler) removeOrphanedPCIDevices(ctx context.Context, deviceName, assignedNamespace string) error {
	var pciDeviceList v1alpha2.PCIDeviceList
	if err := h.client.List(ctx, &pciDeviceList, client.MatchingFields{indexer.IndexFieldPCIDeviceByName: deviceName}); err != nil {
		return fmt.Errorf("failed to list PCIDevices: %w", err)
	}

	for _, pciDevice := range pciDeviceList.Items {
		if assignedNamespace == "" || pciDevice.Namespace != assignedNamespace {
			if err := h.deletePCIDevice(ctx, pciDevice.Namespace, pciDevice.Name); err != nil {
				return fmt.Errorf("failed to delete PCIDevice %s/%s: %w", pciDevice.Namespace, pciDevice.Name, err)
			}
		}
	}

	return nil
}

func (h *AssignedHandler) ensurePCIDevice(ctx context.Context, nodePCIDevice *v1alpha2.NodePCIDevice, namespace string) (*v1alpha2.PCIDevice, error) {
	pciDevice := &v1alpha2.PCIDevice{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      nodePCIDevice.Name,
	}

	err := h.client.Get(ctx, key, pciDevice)
	if err == nil {
		if !pciDevice.GetDeletionTimestamp().IsZero() {
			return nil, errPCIDeviceTerminating
		}
		if !equality.Semantic.DeepEqual(pciDevice.Status.Attributes, nodePCIDevice.Status.Attributes) || pciDevice.Status.NodeName != nodePCIDevice.Status.NodeName {
			pciDevice.Status.Attributes = nodePCIDevice.Status.Attributes
			pciDevice.Status.NodeName = nodePCIDevice.Status.NodeName
			if err := h.client.Status().Update(ctx, pciDevice); err != nil {
				return nil, fmt.Errorf("failed to update PCIDevice status: %w", err)
			}
		}
		return pciDevice, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get PCIDevice: %w", err)
	}

	// PCIDevice doesn't exist - create it
	pciDevice = &v1alpha2.PCIDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodePCIDevice.Name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.NodePCIDeviceKind,
					Name:       nodePCIDevice.Name,
					UID:        nodePCIDevice.UID,
					Controller: ptr.To(true),
				},
			},
		},
		Status: v1alpha2.PCIDeviceStatus{
			Attributes: nodePCIDevice.Status.Attributes,
			NodeName:   nodePCIDevice.Status.NodeName,
		},
	}

	if err := h.client.Create(ctx, pciDevice); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := h.client.Get(ctx, key, pciDevice); err != nil {
				return nil, fmt.Errorf("failed to get existing PCIDevice: %w", err)
			}
			if !equality.Semantic.DeepEqual(pciDevice.Status.Attributes, nodePCIDevice.Status.Attributes) || pciDevice.Status.NodeName != nodePCIDevice.Status.NodeName {
				pciDevice.Status.Attributes = nodePCIDevice.Status.Attributes
				pciDevice.Status.NodeName = nodePCIDevice.Status.NodeName
				if err := h.client.Status().Update(ctx, pciDevice); err != nil {
					return nil, fmt.Errorf("failed to update PCIDevice status: %w", err)
				}
			}
			return pciDevice, nil
		}
		return nil, fmt.Errorf("failed to create PCIDevice: %w", err)
	}

	return pciDevice, nil
}

func (h *AssignedHandler) deletePCIDevice(ctx context.Context, namespace, name string) error {
	pciDevice := &v1alpha2.PCIDevice{}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	err := h.client.Get(ctx, key, pciDevice)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// PCIDevice doesn't exist - nothing to delete
			return nil
		}
		return fmt.Errorf("failed to get PCIDevice: %w", err)
	}

	if err := h.client.Delete(ctx, pciDevice); err != nil {
		if isNotFoundPCIDeviceError(err, namespace, name) {
			return nil
		}
		return fmt.Errorf("failed to delete PCIDevice: %w", err)
	}

	return nil
}

func isNotFoundPCIDeviceError(err error, namespace, name string) bool {
	if apierrors.IsNotFound(err) {
		return true
	}

	errText := err.Error()
	return namespace != "" && strings.Contains(errText, "pcidevices.virtualization.deckhouse.io") && strings.Contains(errText, name) && strings.Contains(errText, "not found")
}
