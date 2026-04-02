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

	if progress < syncRangeMin || progress > syncRangeMax {
		t.Fatalf("expected progress in sync range [%d,%d], got=%d", syncRangeMin, syncRangeMax, progress)
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
