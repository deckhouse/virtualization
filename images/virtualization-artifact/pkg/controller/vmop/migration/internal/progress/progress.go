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

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
)

const (
	SyncRangeMin int32 = 10
	SyncRangeMax int32 = 90

	bulkCeiling      = 48.0
	iterativeFloor   = 46.0
	iterativeCeiling = 90.0
	bulkStallWindow  = 45 * time.Second
	iterStallWindow  = 25 * time.Second
)

type Strategy interface {
	SyncProgress(record Record) int32
	Forget(uid types.UID)
}

type Record struct {
	OperationUID         types.UID
	Now                  time.Time
	StartedAt            time.Time
	PreviousProgress     int32
	Phase                virtv1.VirtualMachineInstanceMigrationPhase
	Mode                 virtv1.MigrationMode
	HasIteration         bool
	Iteration            uint32
	HasThrottle          bool
	AutoConvergeThrottle uint32
	Throttle             float64
	DataTotalMiB         float64
	DataProcessedMiB     float64
	DataRemainingMiB     float64
}

type Progress struct {
	store *Store
}

func NewProgress() *Progress {
	return &Progress{store: NewStore()}
}

func (p *Progress) Forget(uid types.UID) {
	if p == nil || p.store == nil || uid == "" {
		return
	}
	p.store.Delete(uid)
}

func (p *Progress) SyncProgress(record Record) int32 {
	state := p.getState(record)
	prev := clampSyncRange(record.PreviousProgress)
	if state.Progress > prev {
		prev = state.Progress
	}

	elapsed := record.Now.Sub(record.StartedAt)
	if elapsed < 0 {
		elapsed = 0
	}

	iterative := isIterative(record)
	if iterative && !state.Iterative {
		state.Iterative = true
		state.IterativeSince = record.Now
		if prev < int32(iterativeFloor) {
			prev = int32(iterativeFloor)
		}
	}

	target := bulkTarget(record, elapsed)
	if iterative {
		target = iterativeTarget(record, state, prev)
	}

	next := smoothProgress(prev, target, iterative, record.Throttle)
	next = applyStatefulStall(record, state, next, iterative)
	next = clampSyncRange(maxInt32(prev, next))

	updateMetricState(record, state)
	state.Progress = next
	state.LastUpdatedAt = record.Now
	state.LastIteration = record.Iteration
	state.Iterative = iterative

	if record.OperationUID != "" {
		p.store.Store(record.OperationUID, state)
	}

	return next
}

func (p *Progress) getState(record Record) State {
	if p == nil || p.store == nil || record.OperationUID == "" {
		return State{Progress: clampSyncRange(record.PreviousProgress), LastMetricAt: record.Now}
	}
	state, ok := p.store.Load(record.OperationUID)
	if !ok {
		state = State{Progress: clampSyncRange(record.PreviousProgress), LastMetricAt: record.Now}
	}
	return state
}

func metricPercent(record Record) (float64, bool) {
	if record.DataTotalMiB <= 0 {
		return 0, false
	}

	processed, hasProcessed := normalizedProcessedMiB(record)
	if !hasProcessed {
		return 0, false
	}

	return clampFloat((processed/record.DataTotalMiB)*100.0, 0, 100), true
}

func normalizedProcessedMiB(record Record) (float64, bool) {
	if record.DataTotalMiB <= 0 {
		return 0, false
	}
	if record.DataProcessedMiB >= 0 {
		return clampFloat(record.DataProcessedMiB, 0, record.DataTotalMiB), true
	}
	if record.DataRemainingMiB >= 0 {
		return clampFloat(record.DataTotalMiB-record.DataRemainingMiB, 0, record.DataTotalMiB), true
	}
	return 0, false
}

func bulkTarget(record Record, elapsed time.Duration) float64 {
	timeTarget := float64(SyncRangeMin) + math.Min(14, elapsed.Seconds()/8)
	metricPct, hasMetric := metricPercent(record)
	if !hasMetric {
		return clampFloat(timeTarget, float64(SyncRangeMin), bulkCeiling)
	}
	metricTarget := float64(SyncRangeMin) + (metricPct/100.0)*(bulkCeiling-float64(SyncRangeMin))
	mixed := metricTarget*0.78 + timeTarget*0.22
	return clampFloat(mixed, float64(SyncRangeMin), bulkCeiling)
}

func iterativeTarget(record Record, state State, current int32) float64 {
	baseline := math.Max(float64(current), iterativeFloor)
	if record.HasIteration {
		baseline = math.Max(baseline, iterativeFloor+math.Min(float64(record.Iteration), 6)*1.5)
	}

	target := baseline
	metricPct, hasMetric := metricPercent(record)
	if hasMetric {
		target = math.Max(target, iterativeMetricTarget(record, metricPct))
	}

	iterativeSince := state.IterativeSince
	if iterativeSince.IsZero() {
		iterativeSince = record.Now
	}
	iterElapsed := record.Now.Sub(iterativeSince)
	if iterElapsed < 0 {
		iterElapsed = 0
	}
	target += math.Min(10, iterElapsed.Seconds()/12)
	if record.HasThrottle {
		target += record.Throttle * 6
	}
	if !hasMetric {
		target += math.Min(34, iterElapsed.Seconds()/20)
	}

	return clampFloat(target, iterativeFloor, iterativeCeiling)
}

func iterativeMetricTarget(record Record, metricPct float64) float64 {
	if record.DataTotalMiB > 0 && record.DataRemainingMiB >= 0 {
		remainingRatio := clampFloat(record.DataRemainingMiB/record.DataTotalMiB, 0.0001, 1)
		shaped := 1 - math.Log1p(remainingRatio*9)/math.Log(10)
		return clampFloat(iterativeFloor+shaped*(iterativeCeiling-iterativeFloor), iterativeFloor, iterativeCeiling)
	}
	return clampFloat(iterativeFloor+(metricPct/100.0)*(iterativeCeiling-iterativeFloor), iterativeFloor, iterativeCeiling)
}

func isIterative(record Record) bool {
	if record.HasIteration && record.Iteration > 0 {
		return true
	}
	return record.Mode == virtv1.MigrationPostCopy || record.Mode == virtv1.MigrationPaused
}

func smoothProgress(current int32, target float64, iterative bool, throttle float64) int32 {
	delta := target - float64(current)
	if delta <= 0 {
		return current
	}
	factor := 0.40
	if iterative {
		factor = 0.28
	}
	step := math.Max(1, math.Round(delta*factor))
	if iterative && throttle > 0 {
		step += math.Round(throttle * 2)
	}
	return current + int32(step)
}

func applyStatefulStall(record Record, state State, current int32, iterative bool) int32 {
	window := bulkStallWindow
	if iterative {
		window = iterStallWindow
	}
	lastMetricAt := state.LastMetricAt
	if lastMetricAt.IsZero() {
		lastMetricAt = record.Now
	}
	if record.Now.Sub(lastMetricAt) < window {
		return current
	}
	bump := int32(1)
	if iterative && record.HasThrottle && record.Throttle >= 0.5 {
		bump = 2
	}
	return clampSyncRange(current + bump)
}

func updateMetricState(record Record, state State) {
	if !metricChanged(record, state) {
		return
	}
	state.LastMetricAt = record.Now
	state.LastProcessedMiB = record.DataProcessedMiB
	state.LastRemainingMiB = record.DataRemainingMiB
}

func metricChanged(record Record, state State) bool {
	if state.LastMetricAt.IsZero() {
		return true
	}
	if record.DataProcessedMiB >= 0 && !almostEqual(record.DataProcessedMiB, state.LastProcessedMiB) {
		return true
	}
	if record.DataRemainingMiB >= 0 && !almostEqual(record.DataRemainingMiB, state.LastRemainingMiB) {
		return true
	}
	return false
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.01
}

func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
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
	if v < SyncRangeMin {
		return SyncRangeMin
	}
	if v > SyncRangeMax {
		return SyncRangeMax
	}
	return v
}
