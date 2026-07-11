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

package vmbda

import (
	"fmt"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// BeAttached reports the VirtualMachineBlockDeviceAttachment has reached the
// Attached phase. Intended for use with [Observer.WaitFor].
func BeAttached() Predicate {
	return func(vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (bool, error) {
		return vmbda.Status.Phase == v1alpha2.BlockDeviceAttachmentPhaseAttached, nil
	}
}

// BeFailed reports an invariant violation when the attachment enters the
// terminal Failed phase. Intended for use with [Observer.Never].
func BeFailed() Predicate {
	return func(vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (bool, error) {
		if vmbda.Status.Phase == v1alpha2.BlockDeviceAttachmentPhaseFailed {
			return true, fmt.Errorf("VirtualMachineBlockDeviceAttachment entered Failed phase")
		}
		return false, nil
	}
}
