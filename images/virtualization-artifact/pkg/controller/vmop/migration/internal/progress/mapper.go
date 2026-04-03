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

func BuildRecord(vmop *v1alpha2.VirtualMachineOperation, mig *virtv1.VirtualMachineInstanceMigration, autoConverge bool, now time.Time) Record {
	record := Record{
		Now:              now,
		StartedAt:        now,
		PreviousProgress: previousProgress(vmop),
		AutoConverge:     autoConverge,
		DataTotalMiB:     unknownMetric,
		DataProcessedMiB: unknownMetric,
		DataRemainingMiB: unknownMetric,
	}

	if vmop != nil {
		record.OperationUID = vmop.UID
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
		record.Iteration, record.HasIteration = mapIteration(state)
		record.AutoConvergeThrottle, record.HasThrottle = mapThrottle(state)
		record.Throttle = normalizeThrottle(record.AutoConvergeThrottle, record.HasThrottle)
		record.DataTotalMiB = mapBytesToMiB(state.DataTotalBytes)
		record.DataProcessedMiB = mapBytesToMiB(state.DataProcessedBytes)
		record.DataRemainingMiB = mapBytesToMiB(state.DataRemainingBytes)
	}

	return record
}

func mapBytesToMiB(v *uint64) float64 {
	if v == nil {
		return unknownMetric
	}
	return float64(*v) / (1024.0 * 1024.0)
}

func previousProgress(vmop *v1alpha2.VirtualMachineOperation) int32 {
	if vmop == nil || vmop.Status.Progress == nil {
		return SyncRangeMin
	}
	return *vmop.Status.Progress
}

func mapIteration(state *virtv1.VirtualMachineInstanceMigrationState) (uint32, bool) {
	if state == nil || state.Iteration == nil {
		return 0, false
	}
	return *state.Iteration, true
}

func mapThrottle(state *virtv1.VirtualMachineInstanceMigrationState) (uint32, bool) {
	if state == nil || state.AutoConvergeThrottle == nil {
		return 0, false
	}
	return *state.AutoConvergeThrottle, true
}

func normalizeThrottle(raw uint32, ok bool) float64 {
	if !ok {
		return 0
	}
	return clampFloat(float64(raw)/100.0, 0, 1)
}
