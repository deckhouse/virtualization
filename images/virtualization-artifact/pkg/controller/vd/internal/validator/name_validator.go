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
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NameValidator struct{}

func NewNameValidator() *NameValidator {
	return &NameValidator{}
}

func (v *NameValidator) ValidateCreate(_ context.Context, vd *virtv2.VirtualDisk) (admission.Warnings, error) {
	if strings.Contains(vd.ObjectMeta.Name, ".") {
		return nil, errors.New("virtual disk name cannot contain '.'")
	}

	return nil, nil
}

func (v *NameValidator) ValidateUpdate(_ context.Context, _, newVD *virtv2.VirtualDisk) (admission.Warnings, error) {
	if strings.Contains(newVD.ObjectMeta.Name, ".") {
		var warnings admission.Warnings
		warnings = append(warnings, fmt.Sprintf("virtual disk name contain '.', it may be cause of problems in future, please recreate resource."))
		return warnings, nil
	}

	return nil, nil
}
