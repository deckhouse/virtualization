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

package vdsnapshot

import (
	"fmt"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

// BeReady reports the VirtualDiskSnapshot has finished and reached the Ready
// phase. A Failed phase is reported as a definite error so that any WaitFor
// caller fails immediately instead of waiting for the timeout. Intended for use
// with [Observer.WaitFor].
func BeReady() Predicate {
	return func(s *v1alpha2.VirtualDiskSnapshot) (bool, error) {
		switch s.Status.Phase {
		case v1alpha2.VirtualDiskSnapshotPhaseReady:
			return true, nil
		case v1alpha2.VirtualDiskSnapshotPhaseFailed:
			return false, fmt.Errorf("VirtualDiskSnapshot entered Failed phase: %s", failureDetail(s))
		default:
			return false, nil
		}
	}
}

// failureDetail extracts the reason and message the vdsnapshot controller left
// on the VirtualDiskSnapshotReady condition (e.g. the latched VolumeSnapshot
// error), so a Failed phase surfaces its cause instead of a bare phase name.
func failureDetail(s *v1alpha2.VirtualDiskSnapshot) string {
	for _, c := range s.Status.Conditions {
		if c.Type == vdscondition.VirtualDiskSnapshotReadyType.String() {
			return fmt.Sprintf("%s: %s", c.Reason, c.Message)
		}
	}
	return "no VirtualDiskSnapshotReady condition with details found"
}
