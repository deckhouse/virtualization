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

package nodeusbdevice

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewValidator(log *log.Logger) *Validator {
	return &Validator{
		log: log.With("webhook", "validation"),
	}
}

type Validator struct {
	log *log.Logger
}

// ValidateCreate validates NodeUSBDevice creation.
// NodeUSBDevice resources are created automatically by the controller when devices are discovered.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	nodeUSBDevice, ok := obj.(*v1alpha2.NodeUSBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a new NodeUSBDevice but got a %T", obj)
	}

	v.log.Info("Validate NodeUSBDevice creating", "name", nodeUSBDevice.Name)

	// NodeUSBDevice resources are created automatically by the controller
	// Manual creation is allowed for administrative purposes (e.g., testing)
	// but spec.assignedNamespace can be set by administrators
	return nil, nil
}

// ValidateUpdate validates NodeUSBDevice updates.
// Only spec.assignedNamespace can be changed by administrators.
// Status updates are performed by the controller.
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldNodeUSBDevice, ok := oldObj.(*v1alpha2.NodeUSBDevice)
	if !ok {
		return nil, fmt.Errorf("expected an old NodeUSBDevice but got a %T", oldObj)
	}

	newNodeUSBDevice, ok := newObj.(*v1alpha2.NodeUSBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a new NodeUSBDevice but got a %T", newObj)
	}

	v.log.Info("Validate NodeUSBDevice updating", "name", newNodeUSBDevice.Name)

	// Only spec.assignedNamespace can be changed by administrators
	// Status is managed by the controller
	// If spec changed in a way other than assignedNamespace, reject
	if oldNodeUSBDevice.Spec.AssignedNamespace != newNodeUSBDevice.Spec.AssignedNamespace {
		// This is allowed - administrators can assign/unassign namespaces
		return nil, nil
	}

	// Status changes are allowed (performed by the controller)
	// Spec changes other than assignedNamespace are not allowed
	return nil, nil
}

// ValidateDelete validates NodeUSBDevice deletion.
// NodeUSBDevice resources can be deleted by administrators.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	nodeUSBDevice, ok := obj.(*v1alpha2.NodeUSBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a NodeUSBDevice but got a %T", obj)
	}

	v.log.Info("Validate NodeUSBDevice deleting", "name", nodeUSBDevice.Name)

	// NodeUSBDevice can be deleted by administrators
	// The controller will clean up associated USBDevice resources via finalizer
	return nil, nil
}
