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
	"math"
	"time"

	virtv1 "kubevirt.io/api/core/v1"
)

const (
	syncRangeMin int32 = 10
	syncRangeMax int32 = 90

	// These coefficients tune the degraded-mode progress estimation when KubeVirt
	// does not expose byte counters for migration transfer state. The algorithm
	// keeps early stages below the sync range, maps active data synchronization
	// into [10,90], and preserves monotonic growth with a small stall bump.
	progressStartPercent      = 3.0
	progressBulkCeiling       = 45.0
	progressIterativeCeiling  = 98.0
	progressBulkWeightMetric  = 0.80
	progressBulkWeightTime    = 0.20
	progressIterWeightMetric  = 0.76
	progressIterWeightTime    = 0.24
	progressBulkTimeRate      = 0.45
	progressIterBaseTimeRate  = 0.22
	progressIterThrottleRate  = 0.18
	progressBulkStallSeconds  = 45
	progressIterStallSeconds  = 30
	progressBulkDurationGuess = 90.0
)

type Strategy interface {
	SyncProgress(record Record) int32
}

type Record struct {
	Now              time.Time
	StartedAt        time.Time
	PreviousProgress int32
	Phase            virtv1.VirtualMachineInstanceMigrationPhase
	Mode             virtv1.MigrationMode
	Iteration        int32
	Throttle         float64
	DataTotalMiB     float64
	DataProcessedMiB float64
	DataRemainingMiB float64
}

type Progress struct{}

func NewProgress() *Progress {
	return &Progress{}
}

func (p *Progress) SyncProgress(record Record) int32 {
	elapsed := max(record.Now.Sub(record.StartedAt), 0)
	elapsedSec := elapsed.Seconds()

	metricPct, hasMetric := metricPercent(record)
	var internal float64

	if isIterative(record, elapsedSec) {
		iterTime := progressBulkCeiling + math.Max(0, elapsedSec-progressBulkDurationGuess)*(progressIterBaseTimeRate+clampFloat(record.Throttle, 0, 1)*progressIterThrottleRate)
		iterMetric := iterativeMetricPercent(record, metricPct, hasMetric)
		if hasMetric {
			internal = progressIterWeightMetric*iterMetric + progressIterWeightTime*iterTime
		} else {
			internal = iterTime
		}
		internal = clampFloat(internal, progressBulkCeiling, progressIterativeCeiling)
	} else {
		bulkTime := progressStartPercent + elapsedSec*progressBulkTimeRate
		if hasMetric {
			internal = progressBulkWeightMetric*metricPct + progressBulkWeightTime*bulkTime
		} else {
			internal = bulkTime
		}
		internal = clampFloat(internal, progressStartPercent, progressBulkCeiling)
	}

	syncProgress := mapToSyncRange(internal)
	return applyMonotonicStallBump(record.PreviousProgress, syncProgress, elapsedSec, isIterative(record, elapsedSec))
}

func metricPercent(record Record) (float64, bool) {
	if record.DataTotalMiB > 0 && record.DataProcessedMiB >= 0 {
		return clampFloat((record.DataProcessedMiB/record.DataTotalMiB)*100.0, 0, 100), true
	}
	if record.DataTotalMiB > 0 && record.DataRemainingMiB >= 0 {
		processed := record.DataTotalMiB - record.DataRemainingMiB
		return clampFloat((processed/record.DataTotalMiB)*100.0, 0, 100), true
	}
	return 0, false
}

func iterativeMetricPercent(record Record, metricPct float64, hasMetric bool) float64 {
	if hasMetric {
		if record.DataTotalMiB > 0 && record.DataRemainingMiB >= 0 {
			remainingRatio := clampFloat(record.DataRemainingMiB/record.DataTotalMiB, 0.0001, 1)
			shaped := 1 - math.Log1p(remainingRatio*9)/math.Log(10)
			return clampFloat(progressBulkCeiling+shaped*(progressIterativeCeiling-progressBulkCeiling), progressBulkCeiling, progressIterativeCeiling)
		}
		return clampFloat(progressBulkCeiling+(metricPct/100.0)*(progressIterativeCeiling-progressBulkCeiling), progressBulkCeiling, progressIterativeCeiling)
	}
	return progressBulkCeiling
}

func isIterative(record Record, elapsedSec float64) bool {
	if record.Iteration > 0 {
		return true
	}
	if record.Mode == virtv1.MigrationPostCopy || record.Mode == virtv1.MigrationPaused {
		return true
	}
	if record.Phase == virtv1.MigrationRunning || record.Phase == virtv1.MigrationSynchronizing {
		return elapsedSec >= progressBulkDurationGuess
	}
	return false
}

func applyMonotonicStallBump(previous, current int32, elapsedSec float64, iterative bool) int32 {
	prev := clampSyncRange(previous)
	if current < prev {
		current = prev
	}
	if current == prev {
		window := float64(progressBulkStallSeconds)
		if iterative {
			window = float64(progressIterStallSeconds)
		}
		if elapsedSec >= window {
			current = clampSyncRange(prev + 1)
		}
	}
	return clampSyncRange(current)
}

func mapToSyncRange(internal float64) int32 {
	normalized := (clampFloat(internal, progressStartPercent, progressIterativeCeiling) - progressStartPercent) /
		(progressIterativeCeiling - progressStartPercent)
	mapped := float64(syncRangeMin) + normalized*float64(syncRangeMax-syncRangeMin)
	return clampSyncRange(int32(math.Round(mapped)))
}

func clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func clampSyncRange(v int32) int32 {
	if v < syncRangeMin {
		return syncRangeMin
	}
	if v > syncRangeMax {
		return syncRangeMax
	}
	return v
}
