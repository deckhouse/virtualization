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
		record.DataTotalMiB = mapDataTotalMiB(state)
		record.DataProcessedMiB = mapDataProcessedMiB(state)
		record.DataRemainingMiB = mapDataRemainingMiB(state)
		if state.MigrationConfiguration != nil && state.MigrationConfiguration.AllowAutoConverge != nil {
			record.AutoConverge = *state.MigrationConfiguration.AllowAutoConverge
		}
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
	if vmop == nil {
		return SyncRangeMin
	}
	return ParsePercent(vmop.Status.Progress)
}

func mapIteration(state *virtv1.VirtualMachineInstanceMigrationState) (uint32, bool) {
	if state == nil || state.TransferStatus == nil || state.TransferStatus.Iteration == nil {
		return 0, false
	}
	return *state.TransferStatus.Iteration, true
}

func mapThrottle(state *virtv1.VirtualMachineInstanceMigrationState) (uint32, bool) {
	if state == nil || state.TransferStatus == nil || state.TransferStatus.AutoConvergeThrottle == nil {
		return 0, false
	}
	return *state.TransferStatus.AutoConvergeThrottle, true
}

func mapDataTotalMiB(state *virtv1.VirtualMachineInstanceMigrationState) float64 {
	if state == nil || state.TransferStatus == nil {
		return unknownMetric
	}
	return mapBytesToMiB(state.TransferStatus.DataTotalBytes)
}

func mapDataProcessedMiB(state *virtv1.VirtualMachineInstanceMigrationState) float64 {
	if state == nil || state.TransferStatus == nil {
		return unknownMetric
	}
	return mapBytesToMiB(state.TransferStatus.DataProcessedBytes)
}

func mapDataRemainingMiB(state *virtv1.VirtualMachineInstanceMigrationState) float64 {
	if state == nil || state.TransferStatus == nil {
		return unknownMetric
	}
	return mapBytesToMiB(state.TransferStatus.DataRemainingBytes)
}

func normalizeThrottle(raw uint32, ok bool) float64 {
	if !ok {
		return 0
	}
	return clampFloat(float64(raw)/100.0, 0, 1)
}
