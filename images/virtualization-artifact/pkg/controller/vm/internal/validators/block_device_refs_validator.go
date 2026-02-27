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

package validators

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/validate"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type blockDeviceKey struct {
	Kind v1alpha2.BlockDeviceKind
	Name string
}

type BlockDeviceSpecRefsValidator struct{}

func NewBlockDeviceSpecRefsValidator() *BlockDeviceSpecRefsValidator {
	return &BlockDeviceSpecRefsValidator{}
}

func (v *BlockDeviceSpecRefsValidator) validate(vm *v1alpha2.VirtualMachine) error {
	if err := v.noDoubles(vm); err != nil {
		return err
	}

	if err := v.validateBootOrder(vm); err != nil {
		return err
	}

	for _, bdRef := range vm.Spec.BlockDeviceRefs {
		var maxLen int
		switch bdRef.Kind {
		case v1alpha2.DiskDevice:
			maxLen = validate.MaxDiskNameLen
		case v1alpha2.ImageDevice:
			maxLen = validate.MaxVirtualImageNameLen
		case v1alpha2.ClusterImageDevice:
			maxLen = validate.MaxClusterVirtualImageNameLen
		default:
			continue
		}

		if len(bdRef.Name) > maxLen {
			return fmt.Errorf("%s name %q is too long: it must be no more than %d characters", bdRef.Kind, bdRef.Name, maxLen)
		}
	}

	return nil
}

func (v *BlockDeviceSpecRefsValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validate(vm)
}

func (v *BlockDeviceSpecRefsValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validate(newVM)
}

func (v *BlockDeviceSpecRefsValidator) noDoubles(vm *v1alpha2.VirtualMachine) error {
	seen := make(map[blockDeviceKey]struct{}, len(vm.Spec.BlockDeviceRefs))

	for _, bdRef := range vm.Spec.BlockDeviceRefs {
		key := blockDeviceKey{Kind: bdRef.Kind, Name: bdRef.Name}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("cannot specify the same block device reference more than once: %s with name %q has a duplicate reference", bdRef.Kind, bdRef.Name)
		}

		seen[key] = struct{}{}
	}

	return nil
}

func (v *BlockDeviceSpecRefsValidator) validateBootOrder(vm *v1alpha2.VirtualMachine) error {
	seen := make(map[int]string)
	for _, bdRef := range vm.Spec.BlockDeviceRefs {
		if bdRef.BootOrder == nil {
			continue
		}
		if *bdRef.BootOrder < 1 {
			return fmt.Errorf("bootOrder must be >= 1, got %d for %s %q", *bdRef.BootOrder, bdRef.Kind, bdRef.Name)
		}
		if prev, exists := seen[*bdRef.BootOrder]; exists {
			return fmt.Errorf("duplicate bootOrder %d: already used by %s, conflicts with %s %q", *bdRef.BootOrder, prev, bdRef.Kind, bdRef.Name)
		}
		seen[*bdRef.BootOrder] = fmt.Sprintf("%s/%s", bdRef.Kind, bdRef.Name)
	}
	return nil
}
