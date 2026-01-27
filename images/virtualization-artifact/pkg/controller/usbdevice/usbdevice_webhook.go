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

package usbdevice

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

// ValidateCreate validates USBDevice creation.
// USBDevice resources are managed by the controller and should not be created by users.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	usbDevice, ok := obj.(*v1alpha2.USBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a new USBDevice but got a %T", obj)
	}

	v.log.Info("Validate USBDevice creating", "name", usbDevice.Name, "namespace", usbDevice.Namespace)

	// USBDevice resources are created automatically by the controller
	// Users should not create them directly
	return nil, fmt.Errorf("USBDevice resources are managed by the controller and cannot be created manually. Use NodeUSBDevice to assign devices to namespaces")
}

// ValidateUpdate validates USBDevice updates.
// Only status updates are allowed (performed by the controller).
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*v1alpha2.USBDevice)
	if !ok {
		return nil, fmt.Errorf("expected an old USBDevice but got a %T", oldObj)
	}

	newUSBDevice, ok := newObj.(*v1alpha2.USBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a new USBDevice but got a %T", newObj)
	}

	v.log.Info("Validate USBDevice updating", "name", newUSBDevice.Name, "namespace", newUSBDevice.Namespace)

	// USBDevice has no spec, only status
	// Status updates are allowed (performed by the controller)
	// Users should not modify USBDevice resources
	return nil, nil
}

// ValidateDelete validates USBDevice deletion.
// USBDevice resources are managed by the controller and should not be deleted by users.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	usbDevice, ok := obj.(*v1alpha2.USBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a USBDevice but got a %T", obj)
	}

	v.log.Info("Validate USBDevice deleting", "name", usbDevice.Name, "namespace", usbDevice.Namespace)

	// USBDevice resources are deleted automatically by the controller
	// Users should not delete them directly
	return nil, fmt.Errorf("USBDevice resources are managed by the controller and cannot be deleted manually. Modify NodeUSBDevice.spec.assignedNamespace to remove the device from a namespace")
}
