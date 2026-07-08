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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type blockDeviceKey struct {
	Kind v1alpha2.BlockDeviceKind
	Name string
}

type BlockDeviceSpecRefsValidator struct {
	client client.Client
}

func NewBlockDeviceSpecRefsValidator(client client.Client) *BlockDeviceSpecRefsValidator {
	return &BlockDeviceSpecRefsValidator{client: client}
}

func (v *BlockDeviceSpecRefsValidator) validate(vm *v1alpha2.VirtualMachine) error {
	if err := v.noDoubles(vm); err != nil {
		return err
	}

	if err := v.validateBootOrder(vm); err != nil {
		return err
	}

	// The referenced resource's name is validated by its own webhook and bounded
	// by Kubernetes; a reference longer than that simply cannot match any existing
	// resource (the VM stays Pending), so no length check is needed here.

	return nil
}

func (v *BlockDeviceSpecRefsValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validate(vm)
}

func (v *BlockDeviceSpecRefsValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	var warnings admission.Warnings

	// Non-paravirtualized guests keep their disks on the SATA bus, which cannot be
	// hot-plugged, so a block-device change only applies after a restart. CD-ROMs are
	// the exception: they hot-plug on the USB bus (usbstor) regardless of the virtio
	// driver, so a change touching only CD-ROMs is applied live and must not warn.
	if !newVM.Spec.IsParavirtualizationEnabled() && v.hasNonCdromChange(ctx, oldVM, newVM) {
		warnings = append(warnings, "Hot-plugging block devices with enableParavirtualization=false is not supported. Restart the VM to apply changes.")
	}

	return warnings, v.validate(newVM)
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

// hasNonCdromChange reports whether any block device added or removed between
// oldVM and newVM is something other than a CD-ROM. A device whose CD-ROM status
// cannot be resolved is treated as non-CD-ROM, so the restart warning is emitted
// conservatively.
func (v *BlockDeviceSpecRefsValidator) hasNonCdromChange(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) bool {
	for _, key := range changedBlockDevices(oldVM, newVM) {
		if !v.isCdrom(ctx, key, newVM.GetNamespace()) {
			return true
		}
	}
	return false
}

func changedBlockDevices(oldVM, newVM *v1alpha2.VirtualMachine) []blockDeviceKey {
	oldRefs := make(map[blockDeviceKey]struct{}, len(oldVM.Spec.BlockDeviceRefs))
	for _, bd := range oldVM.Spec.BlockDeviceRefs {
		oldRefs[blockDeviceKey{Kind: bd.Kind, Name: bd.Name}] = struct{}{}
	}
	newRefs := make(map[blockDeviceKey]struct{}, len(newVM.Spec.BlockDeviceRefs))
	for _, bd := range newVM.Spec.BlockDeviceRefs {
		newRefs[blockDeviceKey{Kind: bd.Kind, Name: bd.Name}] = struct{}{}
	}

	var changed []blockDeviceKey
	for key := range oldRefs {
		if _, ok := newRefs[key]; !ok {
			changed = append(changed, key)
		}
	}
	for key := range newRefs {
		if _, ok := oldRefs[key]; !ok {
			changed = append(changed, key)
		}
	}
	return changed
}

func (v *BlockDeviceSpecRefsValidator) isCdrom(ctx context.Context, key blockDeviceKey, namespace string) bool {
	switch key.Kind {
	case v1alpha2.ImageDevice:
		vi, err := object.FetchObject(ctx, types.NamespacedName{Name: key.Name, Namespace: namespace}, v.client, &v1alpha2.VirtualImage{})
		if err != nil || vi == nil {
			return false
		}
		return vi.Status.CDROM
	case v1alpha2.ClusterImageDevice:
		cvi, err := object.FetchObject(ctx, types.NamespacedName{Name: key.Name}, v.client, &v1alpha2.ClusterVirtualImage{})
		if err != nil || cvi == nil {
			return false
		}
		return cvi.Status.CDROM
	default:
		return false
	}
}

func (v *BlockDeviceSpecRefsValidator) validateBootOrder(vm *v1alpha2.VirtualMachine) error {
	seen := make(map[uint]string)
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
