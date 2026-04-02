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

	virtv1 "kubevirt.io/api/core/v1"
)

func TestProgress_MonotonicGrowth(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	first := p.SyncProgress(Record{
		Now:              now,
		StartedAt:        now.Add(-20 * time.Second),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
	})
	second := p.SyncProgress(Record{
		Now:              now,
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
		Now:              now,
		StartedAt:        now.Add(-2 * time.Hour),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		Mode:             virtv1.MigrationPostCopy,
		Iteration:        1,
		Throttle:         1,
		DataTotalMiB:     1024,
		DataProcessedMiB: 2048,
		DataRemainingMiB: 0,
	})

	if progress < SyncRangeMin || progress > SyncRangeMax {
		t.Fatalf("expected progress in sync range [%d,%d], got=%d", SyncRangeMin, SyncRangeMax, progress)
	}
}

func TestProgress_StallBump(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
		Now:              now,
		StartedAt:        now.Add(-50 * time.Second),
		PreviousProgress: 70,
		Phase:            virtv1.MigrationRunning,
	})

	if progress != 71 {
		t.Fatalf("expected stall bump to increase progress to 71, got=%d", progress)
	}
}

func TestProgress_DegradedModeWithoutMetrics(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
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

func TestProgress_WithMetricsInIterativePhase(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	bulk := p.SyncProgress(Record{
		Now:              now,
		StartedAt:        now.Add(-30 * time.Second),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		DataTotalMiB:     1024,
		DataProcessedMiB: 512,
	})
	iterative := p.SyncProgress(Record{
		Now:              now,
		StartedAt:        now.Add(-3 * time.Minute),
		PreviousProgress: bulk,
		Phase:            virtv1.MigrationRunning,
		Mode:             virtv1.MigrationPostCopy,
		Iteration:        1,
		Throttle:         1,
		DataTotalMiB:     1024,
		DataRemainingMiB: 64,
	})

	if iterative <= bulk {
		t.Fatalf("expected iterative progress to be greater than bulk progress, bulk=%d iterative=%d", bulk, iterative)
	}
	if iterative < SyncRangeMin || iterative > SyncRangeMax {
		t.Fatalf("expected iterative progress in sync range [%d,%d], got=%d", SyncRangeMin, SyncRangeMax, iterative)
	}
}

func TestProgress_UsesRemainingDataFallback(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
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
		Now:              now,
		StartedAt:        now,
		PreviousProgress: SyncRangeMin,
		Phase:            virtv1.MigrationPending,
	})

	if progress != SyncRangeMin {
		t.Fatalf("expected zero elapsed progress=%d, got=%d", SyncRangeMin, progress)
	}
}

func TestProgress_VeryLargeElapsedIsCapped(t *testing.T) {
	now := time.Now()
	p := NewProgress()

	progress := p.SyncProgress(Record{
		Now:              now,
		StartedAt:        now.Add(-24 * time.Hour),
		PreviousProgress: 10,
		Phase:            virtv1.MigrationRunning,
		Mode:             virtv1.MigrationPostCopy,
		Iteration:        1,
	})

	if progress != SyncRangeMax {
		t.Fatalf("expected capped progress=%d, got=%d", SyncRangeMax, progress)
	}
}

func TestIsIterative(t *testing.T) {
	tests := []struct {
		name     string
		record   Record
		elapsed  float64
		expected bool
	}{
		{
			name:     "iteration implies iterative",
			record:   Record{Iteration: 1},
			elapsed:  1,
			expected: true,
		},
		{
			name:     "post copy mode implies iterative",
			record:   Record{Mode: virtv1.MigrationPostCopy},
			elapsed:  1,
			expected: true,
		},
		{
			name:     "long running implies iterative",
			record:   Record{Phase: virtv1.MigrationRunning},
			elapsed:  progressBulkDurationGuess,
			expected: true,
		},
		{
			name:     "short pre-copy is not iterative",
			record:   Record{Phase: virtv1.MigrationRunning},
			elapsed:  progressBulkDurationGuess - 1,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIterative(tt.record, tt.elapsed); got != tt.expected {
				t.Fatalf("isIterative() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestProgress_StallBumpNotAppliedEarly(t *testing.T) {
	got := applyMonotonicStallBump(70, 70, float64(progressBulkStallSeconds-1), false)
	if got != 70 {
		t.Fatalf("expected no stall bump before window, got=%d", got)
	}
}

func TestProgress_StallBumpDoesNotRepeatOnRegressedBase(t *testing.T) {
	got := applyMonotonicStallBump(71, 70, float64(progressBulkStallSeconds+10), false)
	if got != 71 {
		t.Fatalf("expected previous progress to be preserved without repeated bump, got=%d", got)
	}
}

func TestMapToSyncRangeBoundaries(t *testing.T) {
	if got := mapToSyncRange(progressStartPercent); got != SyncRangeMin {
		t.Fatalf("expected lower boundary=%d, got=%d", SyncRangeMin, got)
	}
	if got := mapToSyncRange(progressIterativeCeiling); got != SyncRangeMax {
		t.Fatalf("expected upper boundary=%d, got=%d", SyncRangeMax, got)
	}
}
