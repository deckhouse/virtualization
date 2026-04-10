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
	"k8s.io/apimachinery/pkg/types"
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
		ObjectMeta: metav1.ObjectMeta{
			UID:               types.UID("vmop-uid"),
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Minute)),
		},
		Status: v1alpha2.VirtualMachineOperationStatus{Progress: "42%"},
	}

	record := BuildRecord(vmop, nil, now)

	if record.OperationUID != vmop.UID {
		t.Fatalf("expected OperationUID=%s, got %s", vmop.UID, record.OperationUID)
	}
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
	totalBytes := uint64(1024 * 1024 * 1024)
	processedBytes := uint64(512 * 1024 * 1024)
	remainingBytes := uint64(256 * 1024 * 1024)
	iteration := uint32(10)
	autoConvergeThrottle := uint32(50)
	mig := &virtv1.VirtualMachineInstanceMigration{
		Status: virtv1.VirtualMachineInstanceMigrationStatus{
			Phase: virtv1.MigrationRunning,
			MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
				StartTimestamp: &start,
				Mode:           virtv1.MigrationPreCopy,
				TransferStatus: &virtv1.VirtualMachineInstanceMigrationTransferStatus{
					Iteration:            &iteration,
					AutoConvergeThrottle: &autoConvergeThrottle,
					DataTotalBytes:       &totalBytes,
					DataProcessedBytes:   &processedBytes,
					DataRemainingBytes:   &remainingBytes,
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
	if record.Mode != virtv1.MigrationPreCopy {
		t.Fatalf("expected Mode=%s, got %s", virtv1.MigrationPreCopy, record.Mode)
	}
	if !record.HasIteration || record.Iteration != 10 {
		t.Fatalf("expected Iteration=10 with flag, got value=%d has=%v", record.Iteration, record.HasIteration)
	}
	if !record.HasThrottle || record.AutoConvergeThrottle != 50 {
		t.Fatalf("expected AutoConvergeThrottle=50 with flag, got value=%d has=%v", record.AutoConvergeThrottle, record.HasThrottle)
	}
	if record.Throttle != 0.5 {
		t.Fatalf("expected normalized Throttle=0.5, got %v", record.Throttle)
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
			vmop: &v1alpha2.VirtualMachineOperation{Status: v1alpha2.VirtualMachineOperationStatus{Progress: "37%"}},
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
		name    string
		state   *virtv1.VirtualMachineInstanceMigrationState
		want    uint32
		wantSet bool
	}{
		{
			name:    "nil state",
			state:   nil,
			want:    0,
			wantSet: false,
		},
		{
			name:    "missing iteration",
			state:   &virtv1.VirtualMachineInstanceMigrationState{},
			want:    0,
			wantSet: false,
		},
		{
			name: "explicit iteration",
			state: &virtv1.VirtualMachineInstanceMigrationState{TransferStatus: &virtv1.VirtualMachineInstanceMigrationTransferStatus{
				Iteration: ptr.To[uint32](7),
			}},
			want:    7,
			wantSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotSet := mapIteration(tt.state)
			if got != tt.want || gotSet != tt.wantSet {
				t.Fatalf("mapIteration() = (%d,%v), want (%d,%v)", got, gotSet, tt.want, tt.wantSet)
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
		name      string
		state     *virtv1.VirtualMachineInstanceMigrationState
		wantRaw   uint32
		wantSet   bool
		wantValue float64
	}{
		{
			name:      "nil state",
			state:     nil,
			wantRaw:   0,
			wantSet:   false,
			wantValue: 0,
		},
		{
			name:      "missing throttle",
			state:     &virtv1.VirtualMachineInstanceMigrationState{},
			wantRaw:   0,
			wantSet:   false,
			wantValue: 0,
		},
		{
			name: "explicit throttle",
			state: &virtv1.VirtualMachineInstanceMigrationState{TransferStatus: &virtv1.VirtualMachineInstanceMigrationTransferStatus{
				AutoConvergeThrottle: ptr.To[uint32](70),
			}},
			wantRaw:   70,
			wantSet:   true,
			wantValue: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, gotSet := mapThrottle(tt.state)
			if raw != tt.wantRaw || gotSet != tt.wantSet {
				t.Fatalf("mapThrottle() = (%d,%v), want (%d,%v)", raw, gotSet, tt.wantRaw, tt.wantSet)
			}
			if got := normalizeThrottle(raw, gotSet); got != tt.wantValue {
				t.Fatalf("normalizeThrottle() = %v, want %v", got, tt.wantValue)
			}
		})
	}
}

func TestBuildRecord_AutoConvergeFromMigrationConfiguration(t *testing.T) {
	now := time.Unix(1710000000, 0)
	allowAutoConverge := true
	mig := &virtv1.VirtualMachineInstanceMigration{
		Status: virtv1.VirtualMachineInstanceMigrationStatus{
			MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
				MigrationConfiguration: &virtv1.MigrationConfiguration{
					AllowAutoConverge: &allowAutoConverge,
				},
			},
		},
	}

	record := BuildRecord(nil, mig, now)
	if !record.AutoConverge {
		t.Fatal("expected AutoConverge=true from MigrationConfiguration.AllowAutoConverge")
	}
}

func TestBuildRecord_AutoConverge_False_WhenNotSet(t *testing.T) {
	now := time.Unix(1710000000, 0)

	recordNoMig := BuildRecord(nil, nil, now)
	if recordNoMig.AutoConverge {
		t.Fatal("expected AutoConverge=false when mig is nil")
	}

	migNoConfig := &virtv1.VirtualMachineInstanceMigration{
		Status: virtv1.VirtualMachineInstanceMigrationStatus{
			MigrationState: &virtv1.VirtualMachineInstanceMigrationState{},
		},
	}
	recordNoConfig := BuildRecord(nil, migNoConfig, now)
	if recordNoConfig.AutoConverge {
		t.Fatal("expected AutoConverge=false when MigrationConfiguration is nil")
	}

	allowAutoConverge := false
	migFalse := &virtv1.VirtualMachineInstanceMigration{
		Status: virtv1.VirtualMachineInstanceMigrationStatus{
			MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
				MigrationConfiguration: &virtv1.MigrationConfiguration{
					AllowAutoConverge: &allowAutoConverge,
				},
			},
		},
	}
	recordFalse := BuildRecord(nil, migFalse, now)
	if recordFalse.AutoConverge {
		t.Fatal("expected AutoConverge=false when AllowAutoConverge=false")
	}
}
