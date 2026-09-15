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

package v1alpha2

import "testing"

// A snapshot records what a disk restored from it should look like in two places, and which one is
// filled says how the snapshot came to be: a capture fills this module's own fields, an import fills
// only the core's status.data.
func TestVirtualDiskSnapshot_CapturedSizeAndStorageClass(t *testing.T) {
	withData := func(vds *VirtualDiskSnapshot, size, class string) *VirtualDiskSnapshot {
		vds.Status.Data = &UnifiedSnapshotterDataBinding{Size: size, StorageClassName: class}
		return vds
	}

	tests := []struct {
		name      string
		snapshot  *VirtualDiskSnapshot
		wantSize  string
		wantClass string
	}{
		{
			name:     "nil",
			snapshot: nil,
		},
		{
			name:     "still capturing: nothing recorded anywhere",
			snapshot: &VirtualDiskSnapshot{},
		},
		{
			name: "a capture, which fills this module's own fields",
			snapshot: &VirtualDiskSnapshot{Status: VirtualDiskSnapshotStatus{
				PersistentVolumeClaimSize: "64Mi",
				StorageClassName:          "sc-captured",
			}},
			wantSize:  "64Mi",
			wantClass: "sc-captured",
		},
		{
			// The case the fallback exists for: an import captures nothing, so only the core's
			// status.data knows how big the materialized content is and where it lives.
			name:      "an import, which fills only the core's data binding",
			snapshot:  withData(&VirtualDiskSnapshot{}, "69580Ki", "sc-imported"),
			wantSize:  "69580Ki",
			wantClass: "sc-imported",
		},
		{
			// A capture fills both, and they agree; the domain fields win so the answer stays the
			// VirtualDisk's own declared size rather than the artifact's rounded one.
			name: "both recorded: this module's fields win",
			snapshot: withData(&VirtualDiskSnapshot{Status: VirtualDiskSnapshotStatus{
				PersistentVolumeClaimSize: "64Mi",
				StorageClassName:          "sc-captured",
			}}, "69580Ki", "sc-imported"),
			wantSize:  "64Mi",
			wantClass: "sc-captured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.snapshot.CapturedPersistentVolumeClaimSize(); got != tt.wantSize {
				t.Errorf("CapturedPersistentVolumeClaimSize() = %q, want %q", got, tt.wantSize)
			}
			if got := tt.snapshot.CapturedStorageClassName(); got != tt.wantClass {
				t.Errorf("CapturedStorageClassName() = %q, want %q", got, tt.wantClass)
			}
		})
	}
}
