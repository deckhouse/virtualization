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

package progress

import (
	"time"

	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const unknownMetric = -1.0

// BuildRecord maps KubeVirt migration status to progress algorithm inputs.
//
// KubeVirt v1.6 does not expose transferred/remaining byte counters in
// VirtualMachineInstanceMigrationStatus, therefore data metrics are mapped to
// unknown values and Progress runs in deterministic degraded mode
// (time+phase based with stall bump).
func BuildRecord(vmop *v1alpha2.VirtualMachineOperation, mig *virtv1.VirtualMachineInstanceMigration, now time.Time) Record {
	record := Record{
		Now:              now,
		StartedAt:        now,
		PreviousProgress: previousProgress(vmop),
		DataTotalMiB:     unknownMetric,
		DataProcessedMiB: unknownMetric,
		DataRemainingMiB: unknownMetric,
	}

	if vmop != nil {
		record.StartedAt = vmop.CreationTimestamp.Time
	}

	if mig == nil {
		return record
	}

	record.Phase = mig.Status.Phase
	if state := mig.Status.MigrationState; state != nil {
		if state.StartTimestamp != nil {
			record.StartedAt = state.StartTimestamp.Time
		}
		record.Mode = state.Mode
		record.Iteration = mapIteration(state)
		record.Throttle = mapThrottle(state)
	}

	return record
}

func previousProgress(vmop *v1alpha2.VirtualMachineOperation) int32 {
	if vmop == nil || vmop.Status.Progress == nil {
		return syncRangeMin
	}
	return *vmop.Status.Progress
}

// mapIteration approximates iterative phase: post-copy and paused modes are
// treated as iterative (>0), otherwise pre-copy stays at iteration 0.
func mapIteration(state *virtv1.VirtualMachineInstanceMigrationState) int32 {
	if state == nil {
		return 0
	}
	if state.Mode == virtv1.MigrationPostCopy || state.Mode == virtv1.MigrationPaused {
		return 1
	}
	return 0
}

// mapThrottle provides deterministic throttle approximation from available
// flags: auto-converge implies elevated throttle, post-copy/paused implies max.
func mapThrottle(state *virtv1.VirtualMachineInstanceMigrationState) float64 {
	if state == nil {
		return 0
	}
	throttle := 0.0
	if state.MigrationConfiguration != nil && state.MigrationConfiguration.AllowAutoConverge != nil && *state.MigrationConfiguration.AllowAutoConverge {
		throttle = 0.7
	}
	if state.Mode == virtv1.MigrationPostCopy || state.Mode == virtv1.MigrationPaused {
		throttle = 1.0
	}
	return throttle
}
