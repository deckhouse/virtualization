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
	_ = log
	return &Validator{}
}

type Validator struct{}

// ValidateCreate is intentionally a no-op.
// Admission restrictions for CREATE are enforced by policy/RBAC; this webhook handles UPDATE only.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
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

	if !reflect.DeepEqual(oldNodeUSBDevice.Status, newNodeUSBDevice.Status) {
		return nil, fmt.Errorf("status cannot be changed via main resource update")
	}

	if !reflect.DeepEqual(oldNodeUSBDevice.Labels, newNodeUSBDevice.Labels) ||
		!reflect.DeepEqual(oldNodeUSBDevice.Annotations, newNodeUSBDevice.Annotations) ||
		!reflect.DeepEqual(oldNodeUSBDevice.Finalizers, newNodeUSBDevice.Finalizers) ||
		!reflect.DeepEqual(oldNodeUSBDevice.OwnerReferences, newNodeUSBDevice.OwnerReferences) {
		return nil, fmt.Errorf("metadata changes are not allowed")
	}

	oldSpec := oldNodeUSBDevice.Spec
	oldSpec.AssignedNamespace = newNodeUSBDevice.Spec.AssignedNamespace
	if !reflect.DeepEqual(oldSpec, newNodeUSBDevice.Spec) {
		return nil, fmt.Errorf("only spec.assignedNamespace can be changed")
	}

	return nil, nil
}

// ValidateDelete is intentionally a no-op.
// Admission restrictions for DELETE are enforced by policy/RBAC; this webhook handles UPDATE only.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
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
