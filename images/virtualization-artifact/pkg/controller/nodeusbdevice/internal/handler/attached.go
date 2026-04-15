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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

const nameAttachedHandler = "AttachedHandler"

func NewAttachedHandler(client client.Client) *AttachedHandler {
	return &AttachedHandler{client: client}
}

type AttachedHandler struct {
	client client.Client
}

func (h *AttachedHandler) Name() string {
	return nameAttachedHandler
}

func (h *AttachedHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
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
	if assignedNamespace == "" {
		setAttachedCondition(current, &changed.Status.Conditions, metav1.ConditionFalse, nodeusbdevicecondition.AttachedAvailable, "Device is not assigned to any namespace and is not attached to a virtual machine.")
		return reconcile.Result{}, nil
	}

	usbDevice := &v1alpha2.USBDevice{}
	err := h.client.Get(ctx, types.NamespacedName{Namespace: assignedNamespace, Name: current.Name}, usbDevice)
	if err != nil {
		if errors.IsNotFound(err) {
			setAttachedCondition(current, &changed.Status.Conditions, metav1.ConditionFalse, nodeusbdevicecondition.AttachedAvailable, fmt.Sprintf("Corresponding USBDevice %s/%s not found.", assignedNamespace, current.Name))
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, fmt.Errorf("failed to get USBDevice %s/%s: %w", assignedNamespace, current.Name, err)
	}

	attachedCondition := meta.FindStatusCondition(usbDevice.Status.Conditions, string(usbdevicecondition.AttachedType))
	if attachedCondition == nil {
		setAttachedCondition(current, &changed.Status.Conditions, metav1.ConditionFalse, nodeusbdevicecondition.AttachedAvailable, fmt.Sprintf("Attached condition not found in USBDevice %s/%s.", usbDevice.Namespace, usbDevice.Name))
		return reconcile.Result{}, nil
	}

	setAttachedCondition(
		current,
		&changed.Status.Conditions,
		attachedCondition.Status,
		mapAttachedReason(attachedCondition.Reason),
		attachedCondition.Message,
	)

	return reconcile.Result{}, nil
}

func mapAttachedReason(reason string) nodeusbdevicecondition.AttachedReason {
	switch reason {
	case string(usbdevicecondition.AttachedToVirtualMachine):
		return nodeusbdevicecondition.AttachedToVirtualMachine
	case string(usbdevicecondition.DetachedForMigration):
		return nodeusbdevicecondition.DetachedForMigration
	case string(usbdevicecondition.NoFreeUSBIPPort):
		return nodeusbdevicecondition.NoFreeUSBIPPort
	default:
		return nodeusbdevicecondition.AttachedAvailable
	}
}
