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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// LegacyOSValidator rejects attachments to virtual machines with the Legacy osType and
// paravirtualization off: those disks live on an IDE bus, which cannot be hot-plugged,
// and the user has asked for no virtio devices, so handing them the virtio-scsi bus a
// hot-plugged disk uses would contradict that. Without this check the attachment would
// stay Pending forever with no way for the user to tell why.
//
// With paravirtualization on the attachment is allowed. The osType picks a chipset, not
// a guest operating system: whoever enables the flag states that virtio drivers are
// installed, and a guest that can drive virtio-scsi gets its hot-plugged disk like any
// other osType. One that cannot — Windows XP and Server 2003 have no virtio-scsi driver
// at all — simply will not see the disk, which is the same caveat the documentation
// already carries for every VirtualMachineBlockDeviceAttachment.
type LegacyOSValidator struct {
	attacher *service.AttachmentService
}

func NewLegacyOSValidator(attacher *service.AttachmentService) *LegacyOSValidator {
	return &LegacyOSValidator{attacher: attacher}
}

func (v *LegacyOSValidator) ValidateCreate(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	vm, err := v.attacher.GetVirtualMachine(ctx, vmbda.Spec.VirtualMachineName, vmbda.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine %q: %w", vmbda.Spec.VirtualMachineName, err)
	}
	if vm == nil || vm.Spec.OsType != v1alpha2.LegacyOs || vm.Spec.IsParavirtualizationEnabled() {
		return nil, nil
	}

	return nil, fmt.Errorf(
		"unable to attach block device to VirtualMachine %q: hot-plugging is not available for the %s osType with enableParavirtualization=false, its disks are on the IDE bus. Add the block device to the VirtualMachine `.spec.blockDeviceRefs` and restart it",
		vmbda.Spec.VirtualMachineName, v1alpha2.LegacyOs,
	)
}

func (v *LegacyOSValidator) ValidateUpdate(_ context.Context, _, _ *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	return nil, nil
}
