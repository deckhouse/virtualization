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
	"errors"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NameValidator struct{}

func NewNameValidator() *NameValidator {
	return &NameValidator{}
}

func (v *NameValidator) ValidateCreate(_ context.Context, vd *virtv2.VirtualDisk) (admission.Warnings, error) {
	if strings.Contains(vd.ObjectMeta.Name, ".") {
		return nil, errors.New("VirtualDisk name is invalid: '.' is forbidden, allowed name symbols are [0-9a-zA-Z-]")
	}

	return nil, nil
}

func (v *NameValidator) ValidateUpdate(_ context.Context, _, newVD *virtv2.VirtualDisk) (admission.Warnings, error) {
	if strings.Contains(newVD.ObjectMeta.Name, ".") {
		var warnings admission.Warnings
		warnings = append(warnings, "VirtualDisk name is invalid as it contains now forbidden symbol '.', allowed symbols for name are [0-9a-zA-Z-]. Create another disk with valid name to avoid problems with future updates.")
		return warnings, nil
	}

	return nil, nil
}
