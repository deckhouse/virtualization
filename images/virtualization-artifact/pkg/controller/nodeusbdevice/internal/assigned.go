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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
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

func (h *AssignedHandler) Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error) {
	nodeUSBDevice := s.NodeUSBDevice()

	if nodeUSBDevice.IsEmpty() {
		return reconcile.Result{}, nil
	}

	changed := nodeUSBDevice.Changed()

	assignedNamespace := changed.Spec.AssignedNamespace

	// Update Assigned condition
	var reason nodeusbdevicecondition.AssignedReason
	var message string
	var status metav1.ConditionStatus

	if assignedNamespace != "" {
		// TODO: When USBDevice resource is defined, create/check it here
		// For now, just mark as Assigned when namespace is set
		reason = nodeusbdevicecondition.Assigned
		message = fmt.Sprintf("Namespace %s is assigned for the device", assignedNamespace)
		status = metav1.ConditionTrue
	} else {
		reason = nodeusbdevicecondition.Available
		message = "No namespace is assigned for the device"
		status = metav1.ConditionFalse
	}

	cb := conditions.NewConditionBuilder(nodeusbdevicecondition.AssignedType).
		Generation(changed.Generation).
		Status(status).
		Reason(reason).
		Message(message)

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

// TODO: Implement USBDevice creation/deletion when USBDevice resource is defined

func (h *AssignedHandler) Name() string {
	return nameAssignedHandler
}
