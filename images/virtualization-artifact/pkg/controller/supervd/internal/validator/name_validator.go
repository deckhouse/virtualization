/*
Copyright 2024 Flant JSC

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

package validator

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/validate"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NameValidator struct{}

func NewNameValidator() *NameValidator {
	return &NameValidator{}
}

func (v *NameValidator) ValidateCreate(_ context.Context, vd *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if strings.Contains(vd.Name, ".") {
		return nil, fmt.Errorf("the VirtualDisk name %q is invalid: '.' is forbidden, allowed name symbols are [0-9a-zA-Z-]", vd.Name)
	}

	if len(vd.Name) > validate.MaxDiskNameLen {
		return nil, fmt.Errorf("the VirtualDisk name %q is too long: it must be no more than %d characters", vd.Name, validate.MaxDiskNameLen)
	}

	return nil, nil
}

func (v *NameValidator) ValidateUpdate(_ context.Context, _, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	var warnings admission.Warnings

	if strings.Contains(newVD.Name, ".") {
		warnings = append(warnings, fmt.Sprintf("the VirtualDisk name %q is invalid as it contains now forbidden symbol '.', allowed symbols for name are [0-9a-zA-Z-]. Create another disk with valid name to avoid problems with future updates.", newVD.Name))
	}

	if len(newVD.Name) > validate.MaxDiskNameLen {
		warnings = append(warnings, fmt.Sprintf("the VirtualDisk name %q is too long: it must be no more than %d characters", newVD.Name, validate.MaxDiskNameLen))
	}

	return warnings, nil
}
