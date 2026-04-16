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

	bulkCeiling      = 45.0
	iterativeFloor   = 45.0
	iterativeCeiling = 90.0
	thresholdFactor  = 0.05

	notConvergingWindow = 10 * time.Second

	bulkTimeRate      = 0.55
	iterBaseTimeRate  = 0.022
	iterThrottleRate  = 0.0012
	bulkMetricWeight  = 0.80
	bulkTimeWeight    = 0.20
	iterMetricWeight  = 0.76
	iterTimeWeight    = 0.24
	smoothAlphaUp     = 0.18
	smoothAlphaDown   = 0.34
	bulkStallSeconds  = 10.0
	iterStallSeconds  = 8.0
	finalStallSeconds = 6.0
)

type Strategy interface {
	SyncProgress(record Record) int32
	IsNotConverging(record Record) bool
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
	AutoConverge         bool
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
	elapsedSec := elapsed.Seconds()

	iterative := isIterative(record)
	if iterative && !state.Iterative {
		state.Iterative = true
		state.IterativeSince = record.Now
		p.initIterative(record, &state, elapsedSec)
	}

	if iterative {
		observeRemaining(record, &state)
	}

	target := bulkTarget(record, elapsedSec)
	if iterative {
		target = iterativeTarget(record, &state, elapsedSec)
	}

	maxStep := int32(10)
	if iterative {
		maxStep = 5
	}

	cap := stageCap(iterative)
	progress := math.Max(float64(prev), math.Min(target, cap))
	next := clampPercent(progress)
	if next < prev {
		next = prev
	}
	if next > prev+maxStep {
		next = prev + maxStep
	}

	if next == prev && float64(next) < cap {
		lastIncrease := state.LastIncreaseAt
		if lastIncrease.IsZero() {
			lastIncrease = record.StartedAt
		}
		stallWin := stallWindow(record, &state, iterative)
		if record.Now.Sub(lastIncrease).Seconds() >= stallWin {
			next++
		}
	}

	if float64(next) > cap {
		next = int32(cap)
	}

	if next > prev {
		state.LastIncreaseAt = record.Now
	}

	updateMetricState(record, &state)
	updateMinRemaining(record, &state)
	state.Progress = next
	state.LastUpdatedAt = record.Now
	state.LastIteration = record.Iteration
	state.Iterative = iterative

	if record.OperationUID != "" {
		p.store.Store(record.OperationUID, state)
	}

	return next
}

func (p *Progress) IsNotConverging(record Record) bool {
	if p == nil || p.store == nil || record.OperationUID == "" {
		return false
	}

	state, ok := p.store.Load(record.OperationUID)
	if !ok || !state.Iterative {
		return false
	}

	if !isAtMaxThrottle(record) {
		return false
	}

	if state.MinRemaining <= 0 || state.MinRemainingAt.IsZero() {
		return false
	}

	return record.Now.Sub(state.MinRemainingAt) >= notConvergingWindow
}

func (p *Progress) getState(record Record) State {
	if p == nil || p.store == nil || record.OperationUID == "" {
		return State{Progress: clampSyncRange(record.PreviousProgress), LastMetricAt: record.Now}
	}
	state, ok := p.store.Load(record.OperationUID)
	if !ok {
		state = State{
			Progress:     clampSyncRange(record.PreviousProgress),
			LastMetricAt: record.Now,
		}
	}
	return state
}

func (p *Progress) initIterative(record Record, state *State, _ float64) {
	total := record.DataTotalMiB
	if total <= 0 {
		total = 1
	}
	if total > state.InitialTotal {
		state.InitialTotal = total
	}
	if state.InitialTotal <= 0 {
		state.InitialTotal = total
	}

	remaining := maxRemaining(record)
	if remaining <= 0 {
		remaining = state.InitialTotal
	}

	state.Threshold = math.Max(math.Ceil(state.InitialTotal*thresholdFactor), 1)
	state.InitialRemaining = math.Max(remaining, state.Threshold)
	state.SmoothedRemaining = state.InitialRemaining
}

func observeRemaining(record Record, state *State) {
	remaining := maxRemaining(record)
	if remaining <= 0 {
		return
	}

	alpha := smoothAlphaUp
	if remaining < state.SmoothedRemaining {
		alpha = smoothAlphaDown
	}
	if record.Throttle >= 0.80 {
		alpha += 0.08
	}
	if alpha > 0.90 {
		alpha = 0.90
	}

	if state.SmoothedRemaining <= 0 {
		state.SmoothedRemaining = remaining
	} else {
		state.SmoothedRemaining = alpha*remaining + (1-alpha)*state.SmoothedRemaining
	}
}

func bulkTarget(record Record, elapsedSec float64) float64 {
	total := record.DataTotalMiB
	if total <= 0 {
		total = 1
	}

	processed := math.Max(record.DataProcessedMiB, 0)
	metricRatio := clampFloat(processed/total, 0, 1)
	metricPct := float64(SyncRangeMin) + (bulkCeiling-float64(SyncRangeMin))*metricRatio

	timePct := float64(SyncRangeMin) + elapsedSec*bulkTimeRate
	if timePct > bulkCeiling {
		timePct = bulkCeiling
	}

	return bulkMetricWeight*metricPct + bulkTimeWeight*timePct
}

func iterativeTarget(record Record, state *State, elapsedSec float64) float64 {
	metricRatio := iterativeMetricRatio(state)
	metricPct := iterativeFloor + (iterativeCeiling-5-iterativeFloor)*metricRatio

	throttle := record.Throttle
	iterSince := state.IterativeSince
	if iterSince.IsZero() {
		iterSince = record.Now
	}
	iterElapsed := math.Max(0, elapsedSec-record.Now.Sub(iterSince).Seconds()+record.Now.Sub(iterSince).Seconds())
	iterElapsedSec := math.Max(0, record.Now.Sub(iterSince).Seconds())

	timeRate := iterBaseTimeRate + throttle*iterThrottleRate
	timePct := iterativeFloor + iterElapsedSec*timeRate
	if timePct > iterativeCeiling {
		timePct = iterativeCeiling
	}
	_ = iterElapsed

	target := iterMetricWeight*metricPct + iterTimeWeight*timePct
	return math.Min(target, iterativeCeiling)
}

func iterativeMetricRatio(state *State) float64 {
	if state.InitialRemaining <= state.Threshold {
		return 1
	}

	current := math.Max(state.SmoothedRemaining, state.Threshold)
	initial := math.Max(state.InitialRemaining, state.Threshold)
	base := math.Log(initial / state.Threshold)
	if base <= 0 {
		return 1
	}

	ratio := 1 - math.Log(current/state.Threshold)/base
	return clampFloat(ratio, 0, 1)
}

func stageCap(iterative bool) float64 {
	if !iterative {
		return bulkCeiling
	}
	return iterativeCeiling
}

func stallWindow(record Record, state *State, iterative bool) float64 {
	if !iterative {
		return bulkStallSeconds
	}

	if state.Progress >= int32(iterativeCeiling)-2 {
		return 24.0
	}
	if state.Progress >= int32(iterativeCeiling)-5 {
		return 14.0
	}
	if state.SmoothedRemaining > 0 && state.SmoothedRemaining <= state.Threshold {
		return finalStallSeconds
	}

	window := iterStallSeconds - 3*record.Throttle
	if window < finalStallSeconds {
		return finalStallSeconds
	}
	return window
}

func isIterative(record Record) bool {
	return record.HasIteration && record.Iteration > 0
}

func maxRemaining(record Record) float64 {
	if record.DataRemainingMiB > 0 {
		return record.DataRemainingMiB
	}
	if record.DataTotalMiB > 0 && record.DataProcessedMiB >= 0 {
		r := record.DataTotalMiB - record.DataProcessedMiB
		if r > 0 {
			return r
		}
	}
	return 0
}

func updateMinRemaining(record Record, state *State) {
	remaining := maxRemaining(record)
	if remaining <= 0 {
		return
	}
	if state.MinRemaining <= 0 || remaining < state.MinRemaining {
		state.MinRemaining = remaining
		state.MinRemainingAt = record.Now
	}
}

func isAtMaxThrottle(record Record) bool {
	if !record.AutoConverge {
		return true
	}
	return record.HasThrottle && record.Throttle >= 0.99
}

func updateMetricState(record Record, state *State) {
	if !metricChanged(record, state) {
		return
	}
	state.LastMetricAt = record.Now
	state.LastProcessedMiB = record.DataProcessedMiB
	state.LastRemainingMiB = record.DataRemainingMiB
}

func metricChanged(record Record, state *State) bool {
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

func clampPercent(v float64) int32 {
	i := int32(v)
	if i < SyncRangeMin {
		return SyncRangeMin
	}
	if i > SyncRangeMax {
		return SyncRangeMax
	}
	return i
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
