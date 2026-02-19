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

type Validator struct {
	log *log.Logger
}

func NewValidator(log *log.Logger) *Validator {
	return &Validator{
		log: log.With("webhook", "validation"),
	}
}

// ValidateCreate validates USBDevice creation.
// Access control is handled by RBAC - only the controller ServiceAccount has create permissions.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	usbDevice, ok := obj.(*v1alpha2.USBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a new USBDevice but got a %T", obj)
	}

	v.log.Info("Validate USBDevice creating", "name", usbDevice.Name, "namespace", usbDevice.Namespace)

	// RBAC controls access - only the controller ServiceAccount can create USBDevice
	// No additional validation needed here
	return nil, nil
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
// Access control is handled by RBAC - only the controller ServiceAccount has delete permissions.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	usbDevice, ok := obj.(*v1alpha2.USBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a USBDevice but got a %T", obj)
	}

	v.log.Info("Validate USBDevice deleting", "name", usbDevice.Name, "namespace", usbDevice.Namespace)

	// RBAC controls access - only the controller ServiceAccount can delete USBDevice
	// No additional validation needed here
	return nil, nil
}
