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

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodepcidevice/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodepcidevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/pcidevicecondition"
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

func (h *AttachedHandler) Handle(ctx context.Context, s state.NodePCIDeviceState) (reconcile.Result, error) {
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
		setAttachedCondition(current, &changed.Status.Conditions, metav1.ConditionFalse, nodepcidevicecondition.AttachedAvailable, "Device is not assigned to any namespace and is not attached to a virtual machine.")
		return reconcile.Result{}, nil
	}

	pciDevice := &v1alpha2.PCIDevice{}
	err := h.client.Get(ctx, types.NamespacedName{Namespace: assignedNamespace, Name: current.Name}, pciDevice)
	if err != nil {
		if errors.IsNotFound(err) {
			setAttachedCondition(current, &changed.Status.Conditions, metav1.ConditionFalse, nodepcidevicecondition.AttachedAvailable, fmt.Sprintf("Corresponding PCIDevice %s/%s not found.", assignedNamespace, current.Name))
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, fmt.Errorf("failed to get PCIDevice %s/%s: %w", assignedNamespace, current.Name, err)
	}

	attachedCondition := meta.FindStatusCondition(pciDevice.Status.Conditions, string(pcidevicecondition.AttachedType))
	if attachedCondition == nil {
		setAttachedCondition(current, &changed.Status.Conditions, metav1.ConditionFalse, nodepcidevicecondition.AttachedAvailable, fmt.Sprintf("Waiting for the attachment status of PCIDevice %s/%s.", pciDevice.Namespace, pciDevice.Name))
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

func mapAttachedReason(reason string) nodepcidevicecondition.AttachedReason {
	switch reason {
	case string(pcidevicecondition.AttachedToVirtualMachine):
		return nodepcidevicecondition.AttachedToVirtualMachine
	default:
		return nodepcidevicecondition.AttachedAvailable
	}
}
