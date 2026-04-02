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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestBuildRecord_NilVMOPAndMigration(t *testing.T) {
	now := time.Unix(1710000000, 0)

	record := BuildRecord(nil, nil, now)

	if !record.StartedAt.Equal(now) {
		t.Fatalf("expected StartedAt=%v, got %v", now, record.StartedAt)
	}
	if record.PreviousProgress != SyncRangeMin {
		t.Fatalf("expected PreviousProgress=%d, got %d", SyncRangeMin, record.PreviousProgress)
	}
	if record.DataTotalMiB != unknownMetric || record.DataProcessedMiB != unknownMetric || record.DataRemainingMiB != unknownMetric {
		t.Fatalf("expected unknown metrics, got total=%v processed=%v remaining=%v", record.DataTotalMiB, record.DataProcessedMiB, record.DataRemainingMiB)
	}
}

func TestBuildRecord_UsesVMOPCreationTimestampAndPreviousProgress(t *testing.T) {
	now := time.Unix(1710000000, 0)
	vmop := &v1alpha2.VirtualMachineOperation{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Minute))},
		Status:     v1alpha2.VirtualMachineOperationStatus{Progress: ptr.To[int32](42)},
	}

	record := BuildRecord(vmop, nil, now)

	if !record.StartedAt.Equal(vmop.CreationTimestamp.Time) {
		t.Fatalf("expected StartedAt=%v, got %v", vmop.CreationTimestamp.Time, record.StartedAt)
	}
	if record.PreviousProgress != 42 {
		t.Fatalf("expected PreviousProgress=42, got %d", record.PreviousProgress)
	}
}

func TestBuildRecord_UsesMigrationState(t *testing.T) {
	now := time.Unix(1710000000, 0)
	start := metav1.NewTime(now.Add(-5 * time.Minute))
	autoConverge := true
	totalBytes := uint64(1024 * 1024 * 1024)
	processedBytes := uint64(512 * 1024 * 1024)
	remainingBytes := uint64(256 * 1024 * 1024)
	mig := &virtv1.VirtualMachineInstanceMigration{
		Status: virtv1.VirtualMachineInstanceMigrationStatus{
			Phase: virtv1.MigrationRunning,
			MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
				StartTimestamp:     &start,
				Mode:               virtv1.MigrationPostCopy,
				DataTotalBytes:     &totalBytes,
				DataProcessedBytes: &processedBytes,
				DataRemainingBytes: &remainingBytes,
				MigrationConfiguration: &virtv1.MigrationConfiguration{
					AllowAutoConverge: &autoConverge,
				},
			},
		},
	}

	record := BuildRecord(nil, mig, now)

	if record.Phase != virtv1.MigrationRunning {
		t.Fatalf("expected Phase=%s, got %s", virtv1.MigrationRunning, record.Phase)
	}
	if !record.StartedAt.Equal(start.Time) {
		t.Fatalf("expected StartedAt=%v, got %v", start.Time, record.StartedAt)
	}
	if record.Mode != virtv1.MigrationPostCopy {
		t.Fatalf("expected Mode=%s, got %s", virtv1.MigrationPostCopy, record.Mode)
	}
	if record.Iteration != 1 {
		t.Fatalf("expected Iteration=1, got %d", record.Iteration)
	}
	if record.Throttle != 1.0 {
		t.Fatalf("expected Throttle=1.0, got %v", record.Throttle)
	}
	if record.DataTotalMiB != 1024 || record.DataProcessedMiB != 512 || record.DataRemainingMiB != 256 {
		t.Fatalf("expected mapped MiB counters, got total=%v processed=%v remaining=%v", record.DataTotalMiB, record.DataProcessedMiB, record.DataRemainingMiB)
	}
}

func TestPreviousProgress(t *testing.T) {
	tests := []struct {
		name string
		vmop *v1alpha2.VirtualMachineOperation
		want int32
	}{
		{
			name: "nil vmop",
			vmop: nil,
			want: SyncRangeMin,
		},
		{
			name: "nil progress",
			vmop: &v1alpha2.VirtualMachineOperation{},
			want: SyncRangeMin,
		},
		{
			name: "explicit progress",
			vmop: &v1alpha2.VirtualMachineOperation{Status: v1alpha2.VirtualMachineOperationStatus{Progress: ptr.To[int32](37)}},
			want: 37,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := previousProgress(tt.vmop); got != tt.want {
				t.Fatalf("previousProgress() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMapIteration(t *testing.T) {
	tests := []struct {
		name  string
		state *virtv1.VirtualMachineInstanceMigrationState
		want  int32
	}{
		{
			name:  "nil state",
			state: nil,
			want:  0,
		},
		{
			name:  "pre-copy",
			state: &virtv1.VirtualMachineInstanceMigrationState{Mode: virtv1.MigrationPreCopy},
			want:  0,
		},
		{
			name:  "post-copy",
			state: &virtv1.VirtualMachineInstanceMigrationState{Mode: virtv1.MigrationPostCopy},
			want:  1,
		},
		{
			name:  "paused",
			state: &virtv1.VirtualMachineInstanceMigrationState{Mode: virtv1.MigrationPaused},
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapIteration(tt.state); got != tt.want {
				t.Fatalf("mapIteration() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMapBytesToMiB(t *testing.T) {
	if got := mapBytesToMiB(nil); got != unknownMetric {
		t.Fatalf("expected unknown metric for nil, got %v", got)
	}

	bytes := uint64(3 * 1024 * 1024)
	if got := mapBytesToMiB(&bytes); got != 3 {
		t.Fatalf("expected 3 MiB, got %v", got)
	}
}

func TestMapThrottle(t *testing.T) {
	tests := []struct {
		name  string
		state *virtv1.VirtualMachineInstanceMigrationState
		want  float64
	}{
		{
			name:  "nil state",
			state: nil,
			want:  0,
		},
		{
			name:  "default throttle",
			state: &virtv1.VirtualMachineInstanceMigrationState{},
			want:  0,
		},
		{
			name: "auto converge",
			state: &virtv1.VirtualMachineInstanceMigrationState{
				MigrationConfiguration: &virtv1.MigrationConfiguration{AllowAutoConverge: ptr.To(true)},
			},
			want: 0.7,
		},
		{
			name: "post-copy overrides throttle",
			state: &virtv1.VirtualMachineInstanceMigrationState{
				Mode:                   virtv1.MigrationPostCopy,
				MigrationConfiguration: &virtv1.MigrationConfiguration{AllowAutoConverge: ptr.To(true)},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapThrottle(tt.state); got != tt.want {
				t.Fatalf("mapThrottle() = %v, want %v", got, tt.want)
			}
		})
	}
}
