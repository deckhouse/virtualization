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

package validators

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const legacyOSParavirtualizationWarning = "The Legacy osType with enableParavirtualization=true gives the virtual machine " +
	"virtio devices: disks on the virtio-blk bus and a virtio-net adapter. Operating systems of this type have no " +
	"built-in virtio drivers, so unless the drivers are already installed in the guest, the virtual machine will not " +
	"boot. Set enableParavirtualization=false to get the IDE bus and the RTL8139 adapter."

// LegacyOSValidator warns about a Legacy virtual machine left with paravirtualization on.
// enableParavirtualization defaults to true, so a virtual machine created without thinking
// about the field gets devices its guest operating system cannot drive, and the failure is
// visible only inside the guest: the platform reports the virtual machine as running.
type LegacyOSValidator struct{}

func NewLegacyOSValidator() *LegacyOSValidator {
	return &LegacyOSValidator{}
}

func (v *LegacyOSValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.warn(vm), nil
}

func (v *LegacyOSValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.warn(newVM), nil
}

func (v *LegacyOSValidator) warn(vm *v1alpha2.VirtualMachine) admission.Warnings {
	if vm.Spec.OsType != v1alpha2.LegacyOs || !vm.Spec.IsParavirtualizationEnabled() {
		return nil
	}

	return admission.Warnings{legacyOSParavirtualizationWarning}
}
