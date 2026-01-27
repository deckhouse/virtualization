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
	"maps"
	"reflect"

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
// Only spec can be changed by administrators. Metadata cannot be modified.
// Status updates are performed by the controller via subresource.
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

	// TypeMeta cannot be modified
	if !reflect.DeepEqual(oldNodeUSBDevice.TypeMeta, newNodeUSBDevice.TypeMeta) {
		return nil, fmt.Errorf("TypeMeta cannot be changed")
	}

	// Metadata cannot be modified (except fields that Kubernetes manages automatically)
	if oldNodeUSBDevice.Name != newNodeUSBDevice.Name {
		return nil, fmt.Errorf("metadata.name cannot be changed")
	}
	if oldNodeUSBDevice.Namespace != newNodeUSBDevice.Namespace {
		return nil, fmt.Errorf("metadata.namespace cannot be changed")
	}
	if oldNodeUSBDevice.UID != newNodeUSBDevice.UID {
		return nil, fmt.Errorf("metadata.uid cannot be changed")
	}
	if !maps.Equal(oldNodeUSBDevice.Labels, newNodeUSBDevice.Labels) {
		return nil, fmt.Errorf("metadata.labels cannot be changed")
	}
	if !maps.Equal(oldNodeUSBDevice.Annotations, newNodeUSBDevice.Annotations) {
		return nil, fmt.Errorf("metadata.annotations cannot be changed")
	}
	if !reflect.DeepEqual(oldNodeUSBDevice.Finalizers, newNodeUSBDevice.Finalizers) {
		return nil, fmt.Errorf("metadata.finalizers cannot be changed")
	}

	// Status changes are not allowed via main resource update (use /status subresource)
	if !reflect.DeepEqual(oldNodeUSBDevice.Status, newNodeUSBDevice.Status) {
		return nil, fmt.Errorf("status cannot be changed via main resource update, use /status subresource")
	}

	// Only spec can be changed
	// This is allowed - administrators can modify spec (e.g., assignedNamespace)
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
