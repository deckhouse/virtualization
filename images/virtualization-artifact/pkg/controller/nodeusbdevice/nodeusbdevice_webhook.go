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
	"reflect"
	"strings"

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
// NodeUSBDevice resources can only be created by system service accounts (controllers).
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	if isSystemServiceAccount(ctx) {
		return nil, nil
	}
	return nil, fmt.Errorf("NodeUSBDevice can only be created by system service accounts")
}

// ValidateUpdate validates NodeUSBDevice updates.
// Only spec can be changed by administrators. Metadata cannot be modified.
// Status updates are performed by the controller via subresource.
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	// System service accounts can change anything
	if isSystemServiceAccount(ctx) {
		return nil, nil
	}

	oldNodeUSBDevice, ok := oldObj.(*v1alpha2.NodeUSBDevice)
	if !ok {
		return nil, fmt.Errorf("expected an old NodeUSBDevice but got a %T", oldObj)
	}

	newNodeUSBDevice, ok := newObj.(*v1alpha2.NodeUSBDevice)
	if !ok {
		return nil, fmt.Errorf("expected a new NodeUSBDevice but got a %T", newObj)
	}

	// Spec changes are only allowed
	if !reflect.DeepEqual(oldNodeUSBDevice.Spec, newNodeUSBDevice.Spec) {
		return nil, nil
	}

	return nil, fmt.Errorf("only spec.assignedNamespace can be changed")
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

// isSystemServiceAccount checks if the request is made by a system service account.
func isSystemServiceAccount(ctx context.Context) bool {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return false
	}

	if strings.HasPrefix(req.UserInfo.Username, "system:serviceaccount:kube-system:") ||
		strings.HasPrefix(req.UserInfo.Username, "system:serviceaccount:d8-system:") ||
		strings.HasPrefix(req.UserInfo.Username, "system:serviceaccount:d8-virtualization:") {
		return true
	}

	return false
}
