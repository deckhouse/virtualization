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
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
)

func TestProgress_MonotonicGrowth(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	first := p.SyncProgress(Record{
		OperationUID:     types.UID("vmop"),
		Now:              now,
		StartedAt:        now.Add(-20 * time.Second),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
	})
	second := p.SyncProgress(Record{
		OperationUID:     types.UID("vmop"),
		Now:              now.Add(40 * time.Second),
		StartedAt:        now.Add(-80 * time.Second),
		PreviousProgress: first,
		Phase:            virtv1.MigrationRunning,
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
		Mode:                 virtv1.MigrationPostCopy,
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
		PreviousProgress: 70,
		Phase:            virtv1.MigrationRunning,
	})
	progress := p.SyncProgress(Record{
		OperationUID:     uid,
		Now:              now.Add(bulkStallWindow + time.Second),
		StartedAt:        now.Add(-50 * time.Second),
		PreviousProgress: first,
		Phase:            virtv1.MigrationRunning,
	})

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

func TestMetricPercent_ClampsProcessedAboveTotal(t *testing.T) {
	metricPct, hasMetric := metricPercent(Record{DataTotalMiB: 100, DataProcessedMiB: 200})
	if !hasMetric {
		t.Fatal("expected metric to be available")
	}
	if metricPct != 100 {
		t.Fatalf("expected clamped metric percent=100, got=%v", metricPct)
	}
}

func TestMetricPercent_ClampsRemainingAboveTotal(t *testing.T) {
	metricPct, hasMetric := metricPercent(Record{DataTotalMiB: 100, DataRemainingMiB: 200})
	if !hasMetric {
		t.Fatal("expected metric to be available")
	}
	if metricPct != 0 {
		t.Fatalf("expected clamped metric percent=0, got=%v", metricPct)
	}
}

func TestMetricPercent_RequiresPositiveTotal(t *testing.T) {
	if _, hasMetric := metricPercent(Record{DataTotalMiB: 0, DataProcessedMiB: 10}); hasMetric {
		t.Fatal("expected metric to be unavailable for zero total")
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
	})

	if progress < int32(iterativeFloor) || progress > SyncRangeMax {
		t.Fatalf("expected progress in iterative range [%d,%d], got=%d", int32(iterativeFloor), SyncRangeMax, progress)
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
