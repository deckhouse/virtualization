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
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
)

func TestProgress_MonotonicGrowth(t *testing.T) {
	now := time.Now()
	p := NewProgress()
	uid := types.UID("vmop")

	first := p.SyncProgress(Record{
		OperationUID:     uid,
		Now:              now,
		StartedAt:        now.Add(-20 * time.Second),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		DataTotalMiB:     1024,
		DataProcessedMiB: 100,
		DataRemainingMiB: 900,
	})
	second := p.SyncProgress(Record{
		OperationUID:     uid,
		Now:              now.Add(40 * time.Second),
		StartedAt:        now.Add(-80 * time.Second),
		PreviousProgress: first,
		Phase:            virtv1.MigrationRunning,
		DataTotalMiB:     1024,
		DataProcessedMiB: 200,
		DataRemainingMiB: 800,
	})

	if second < first {
		t.Fatalf("expected monotonic progress, first=%d second=%d", first, second)
	}
}

func TestProgress_SyncRangeCaps(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
		OperationUID:         types.UID("vmop"),
		Now:                  now,
		StartedAt:            now.Add(-2 * time.Hour),
		PreviousProgress:     10,
		Phase:                virtv1.MigrationRunning,
		HasIteration:         true,
		Iteration:            1,
		HasThrottle:          true,
		AutoConvergeThrottle: 100,
		Throttle:             1,
		DataTotalMiB:         1024,
		DataProcessedMiB:     2048,
		DataRemainingMiB:     0,
	})

	if progress < SyncRangeMin || progress > SyncRangeMax {
		t.Fatalf("expected progress in sync range [%d,%d], got=%d", SyncRangeMin, SyncRangeMax, progress)
	}
}

func TestProgress_StallBump(t *testing.T) {
	now := time.Now()
	p := NewProgress()
	uid := types.UID("vmop")

	first := p.SyncProgress(Record{
		OperationUID:     uid,
		Now:              now,
		StartedAt:        now.Add(-50 * time.Second),
		PreviousProgress: 30,
		Phase:            virtv1.MigrationRunning,
		DataTotalMiB:     1024,
		DataProcessedMiB: 300,
		DataRemainingMiB: 700,
	})

	var progress int32
	for i := 1; i <= 5; i++ {
		stallDuration := time.Duration(i) * time.Duration(bulkStallSeconds+2) * time.Second
		progress = p.SyncProgress(Record{
			OperationUID:     uid,
			Now:              now.Add(stallDuration),
			StartedAt:        now.Add(-50 * time.Second),
			PreviousProgress: progress,
			Phase:            virtv1.MigrationRunning,
			DataTotalMiB:     1024,
			DataProcessedMiB: 300,
			DataRemainingMiB: 700,
		})
	}

	if progress <= first {
		t.Fatalf("expected stall bump to increase progress beyond %d, got=%d", first, progress)
	}
}

func TestProgress_DegradedModeWithoutMetrics(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
		OperationUID:     types.UID("vmop"),
		Now:              now,
		StartedAt:        now.Add(-2 * time.Minute),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		DataTotalMiB:     unknownMetric,
		DataProcessedMiB: unknownMetric,
		DataRemainingMiB: unknownMetric,
	})

	if progress < SyncRangeMin || progress > SyncRangeMax {
		t.Fatalf("expected degraded-mode progress in sync range [%d,%d], got=%d", SyncRangeMin, SyncRangeMax, progress)
	}
}

func TestProgress_WithMetricsInBulkPhase(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
		OperationUID:     types.UID("vmop"),
		Now:              now,
		StartedAt:        now.Add(-30 * time.Second),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		DataTotalMiB:     1024,
		DataProcessedMiB: 512,
	})

	if progress <= SyncRangeMin || progress >= SyncRangeMax {
		t.Fatalf("expected bulk progress strictly inside sync range, got=%d", progress)
	}
}

func TestProgress_EntersIterativePhaseByIteration(t *testing.T) {
	now := time.Now()
	p := NewProgress()
	uid := types.UID("vmop")

	bulk := p.SyncProgress(Record{
		OperationUID:     uid,
		Now:              now,
		StartedAt:        now.Add(-30 * time.Second),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		DataTotalMiB:     1024,
		DataProcessedMiB: 512,
		DataRemainingMiB: 512,
	})
	iterative := p.SyncProgress(Record{
		OperationUID:         uid,
		Now:                  now.Add(40 * time.Second),
		StartedAt:            now.Add(-3 * time.Minute),
		PreviousProgress:     bulk,
		Phase:                virtv1.MigrationRunning,
		HasIteration:         true,
		Iteration:            2,
		HasThrottle:          true,
		AutoConvergeThrottle: 50,
		Throttle:             0.5,
		DataTotalMiB:         1024,
		DataProcessedMiB:     960,
		DataRemainingMiB:     64,
	})

	if iterative <= bulk {
		t.Fatalf("expected iterative progress to be greater than bulk progress, bulk=%d iterative=%d", bulk, iterative)
	}
}

func TestProgress_UsesRemainingDataFallback(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
		OperationUID:     types.UID("vmop"),
		Now:              now,
		StartedAt:        now.Add(-90 * time.Second),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		DataTotalMiB:     100,
		DataProcessedMiB: unknownMetric,
		DataRemainingMiB: 25,
	})

	if progress <= SyncRangeMin {
		t.Fatalf("expected fallback metric progress above SyncRangeMin, got=%d", progress)
	}
}

func TestProgress_ZeroElapsed(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
		OperationUID:     types.UID("vmop"),
		Now:              now,
		StartedAt:        now,
		PreviousProgress: SyncRangeMin,
		Phase:            virtv1.MigrationPending,
	})

	if progress != SyncRangeMin {
		t.Fatalf("expected zero elapsed progress=%d, got=%d", SyncRangeMin, progress)
	}
}

func TestProgress_VeryLargeElapsedStaysInRange(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
		OperationUID:     types.UID("vmop"),
		Now:              now,
		StartedAt:        now.Add(-24 * time.Hour),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		HasIteration:     true,
		Iteration:        5,
		DataTotalMiB:     1024,
		DataRemainingMiB: 10,
	})

	if progress < SyncRangeMin || progress > SyncRangeMax {
		t.Fatalf("expected progress in range [%d,%d], got=%d", SyncRangeMin, SyncRangeMax, progress)
	}
}

func TestIsIterative(t *testing.T) {
	tests := []struct {
		name     string
		record   Record
		expected bool
	}{
		{
			name:     "iteration implies iterative",
			record:   Record{HasIteration: true, Iteration: 1},
			expected: true,
		},
		{
			name:     "post copy without iteration is not iterative",
			record:   Record{Mode: virtv1.MigrationPostCopy},
			expected: false,
		},
		{
			name:     "paused without iteration is not iterative",
			record:   Record{Mode: virtv1.MigrationPaused},
			expected: false,
		},
		{
			name:     "pre-copy without iteration is not iterative",
			record:   Record{Mode: virtv1.MigrationPreCopy},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIterative(tt.record); got != tt.expected {
				t.Fatalf("isIterative() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestForget_RemovesState(t *testing.T) {
	p := NewProgress()
	uid := types.UID("vmop")
	p.store.Store(uid, State{Progress: 55})

	p.Forget(uid)

	if p.store.Len() != 0 {
		t.Fatalf("expected empty store after forget, got=%d", p.store.Len())
	}
}

func TestProgress_SmoothGrowthOverMultipleSyncs(t *testing.T) {
	now := time.Now()
	p := NewProgress()
	uid := types.UID("vmop")
	start := now.Add(-10 * time.Second)

	var values []int32
	prev := SyncRangeMin
	totalMiB := 1024.0
	remaining := 900.0

	for i := 0; i < 40; i++ {
		tick := now.Add(time.Duration(i*3) * time.Second)
		remaining = math.Max(10, remaining-25)
		processed := totalMiB - remaining

		iter := uint32(0)
		hasIter := false
		if i >= 5 {
			iter = uint32(i - 4)
			hasIter = true
		}

		progress := p.SyncProgress(Record{
			OperationUID:     uid,
			Now:              tick,
			StartedAt:        start,
			PreviousProgress: prev,
			Phase:            virtv1.MigrationRunning,
			HasIteration:     hasIter,
			Iteration:        iter,
			DataTotalMiB:     totalMiB,
			DataProcessedMiB: processed,
			DataRemainingMiB: remaining,
		})

		values = append(values, progress)
		prev = progress
	}

	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1] {
			t.Fatalf("progress decreased at step %d: %d -> %d", i, values[i-1], values[i])
		}
	}

	maxJump := int32(0)
	for i := 1; i < len(values); i++ {
		jump := values[i] - values[i-1]
		if jump > maxJump {
			maxJump = jump
		}
	}
	if maxJump > 15 {
		t.Fatalf("max single-step jump too large: %d (values: %v)", maxJump, values)
	}
}

func TestIterativeMetricRatio(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		wantLow  float64
		wantHigh float64
	}{
		{
			name:     "initial remaining equals threshold",
			state:    State{InitialRemaining: 50, SmoothedRemaining: 50, Threshold: 50},
			wantLow:  0.99,
			wantHigh: 1.01,
		},
		{
			name:     "smoothed at initial",
			state:    State{InitialRemaining: 1000, SmoothedRemaining: 1000, Threshold: 50},
			wantLow:  -0.01,
			wantHigh: 0.01,
		},
		{
			name:     "smoothed at threshold",
			state:    State{InitialRemaining: 1000, SmoothedRemaining: 50, Threshold: 50},
			wantLow:  0.99,
			wantHigh: 1.01,
		},
		{
			name:     "smoothed halfway log scale",
			state:    State{InitialRemaining: 1000, SmoothedRemaining: 200, Threshold: 50},
			wantLow:  0.3,
			wantHigh: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio := iterativeMetricRatio(&tt.state)
			if ratio < tt.wantLow || ratio > tt.wantHigh {
				t.Fatalf("iterativeMetricRatio() = %v, want in [%v, %v]", ratio, tt.wantLow, tt.wantHigh)
			}
		})
	}
}

func TestMaxRemaining(t *testing.T) {
	tests := []struct {
		name   string
		record Record
		want   float64
	}{
		{
			name:   "direct remaining",
			record: Record{DataRemainingMiB: 100},
			want:   100,
		},
		{
			name:   "computed from total minus processed",
			record: Record{DataTotalMiB: 200, DataProcessedMiB: 150},
			want:   50,
		},
		{
			name:   "no data",
			record: Record{DataTotalMiB: unknownMetric, DataProcessedMiB: unknownMetric, DataRemainingMiB: unknownMetric},
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxRemaining(tt.record)
			if !almostEqual(got, tt.want) {
				t.Fatalf("maxRemaining() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObserveRemaining_EMA(t *testing.T) {
	state := State{SmoothedRemaining: 100}

	observeRemaining(Record{DataRemainingMiB: 80}, &state)
	if state.SmoothedRemaining >= 100 || state.SmoothedRemaining <= 80 {
		t.Fatalf("expected EMA to move smoothed remaining between 80 and 100, got=%v", state.SmoothedRemaining)
	}

	prev := state.SmoothedRemaining
	observeRemaining(Record{DataRemainingMiB: 120}, &state)
	if state.SmoothedRemaining <= prev {
		t.Fatalf("expected EMA to increase smoothed remaining from %v, got=%v", prev, state.SmoothedRemaining)
	}
}

func TestProgress_AdaptiveStallWindow(t *testing.T) {
	state := State{Progress: 50, SmoothedRemaining: 100, Threshold: 50}
	record := Record{Throttle: 0}

	w := stallWindow(record, &state, true)
	if w != iterStallSeconds {
		t.Fatalf("expected base iterative stall window=%v, got=%v", iterStallSeconds, w)
	}

	state.Progress = int32(iterativeCeiling) - 2
	w = stallWindow(record, &state, true)
	if w != 24.0 {
		t.Fatalf("expected late-stage stall window=24, got=%v", w)
	}

	state.Progress = int32(iterativeCeiling) - 4
	w = stallWindow(record, &state, true)
	if w != 14.0 {
		t.Fatalf("expected near-end stall window=14, got=%v", w)
	}
}

func makeIterativeState(p *Progress, uid types.UID, now time.Time, minRemaining float64, minRemainingAt time.Time) {
	state := State{
		Progress:          SyncRangeMin,
		Iterative:         true,
		IterativeSince:    now.Add(-30 * time.Second),
		InitialRemaining:  500,
		SmoothedRemaining: minRemaining + 10,
		Threshold:         10,
		MinRemaining:      minRemaining,
		MinRemainingAt:    minRemainingAt,
	}
	p.store.Store(uid, state)
}

func TestIsNotConverging_NoAutoConverge_Stall(t *testing.T) {
	p := NewProgress()
	uid := types.UID("vmop-nc")
	now := time.Now()
	stallStart := now.Add(-15 * time.Second)

	makeIterativeState(p, uid, now, 100.0, stallStart)

	record := Record{
		OperationUID:     uid,
		Now:              now,
		HasIteration:     true,
		Iteration:        3,
		AutoConverge:     false,
		DataRemainingMiB: 100,
	}

	if !p.IsNotConverging(record) {
		t.Fatal("expected IsNotConverging=true when AutoConverge=false, iterative, stall>10s")
	}
}

func TestIsNotConverging_AutoConverge_ThrottleNotMax(t *testing.T) {
	p := NewProgress()
	uid := types.UID("vmop-nc2")
	now := time.Now()
	stallStart := now.Add(-15 * time.Second)

	makeIterativeState(p, uid, now, 100.0, stallStart)

	record := Record{
		OperationUID:     uid,
		Now:              now,
		HasIteration:     true,
		Iteration:        3,
		AutoConverge:     true,
		HasThrottle:      true,
		Throttle:         0.5,
		DataRemainingMiB: 100,
	}

	if p.IsNotConverging(record) {
		t.Fatal("expected IsNotConverging=false when AutoConverge=true and throttle not at max")
	}
}

func TestIsNotConverging_AutoConverge_MaxThrottle_Stall(t *testing.T) {
	p := NewProgress()
	uid := types.UID("vmop-nc3")
	now := time.Now()
	stallStart := now.Add(-15 * time.Second)

	makeIterativeState(p, uid, now, 100.0, stallStart)

	record := Record{
		OperationUID:     uid,
		Now:              now,
		HasIteration:     true,
		Iteration:        3,
		AutoConverge:     true,
		HasThrottle:      true,
		Throttle:         0.99,
		DataRemainingMiB: 100,
	}

	if !p.IsNotConverging(record) {
		t.Fatal("expected IsNotConverging=true when AutoConverge=true, throttle=max, stall>10s")
	}
}

func TestIsNotConverging_RemainingDecreased(t *testing.T) {
	p := NewProgress()
	uid := types.UID("vmop-nc4")
	now := time.Now()

	makeIterativeState(p, uid, now, 50.0, now)

	record := Record{
		OperationUID:     uid,
		Now:              now,
		HasIteration:     true,
		Iteration:        3,
		AutoConverge:     false,
		DataRemainingMiB: 50,
	}

	if p.IsNotConverging(record) {
		t.Fatal("expected IsNotConverging=false when minRemainingAt is just now (stall < 10s)")
	}
}

func TestIsNotConverging_NotIterative(t *testing.T) {
	p := NewProgress()
	uid := types.UID("vmop-nc5")
	now := time.Now()
	stallStart := now.Add(-15 * time.Second)

	state := State{
		Progress:       SyncRangeMin,
		Iterative:      false,
		MinRemaining:   100,
		MinRemainingAt: stallStart,
	}
	p.store.Store(uid, state)

	record := Record{
		OperationUID:     uid,
		Now:              now,
		HasIteration:     false,
		AutoConverge:     false,
		DataRemainingMiB: 100,
	}

	if p.IsNotConverging(record) {
		t.Fatal("expected IsNotConverging=false when not iterative")
	}
}
