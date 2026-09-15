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

package validate

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func objectWith(annotations ...string) metav1.Object {
	meta := &metav1.ObjectMeta{}
	for _, a := range annotations {
		if meta.Annotations == nil {
			meta.Annotations = map[string]string{}
		}
		meta.Annotations[a] = ""
	}
	return meta
}

// An import is assembled by the state-snapshotter core. Accepting one that nothing can assemble would
// turn into a `d8 snapshot restore` waiting on a bind that never comes, so it is refused at creation.
func TestImportModeRequiresUnifiedSnapshotter(t *testing.T) {
	tests := []struct {
		name    string
		obj     metav1.Object
		mode    v1alpha2.UnifiedSnapshotterMode
		present bool
		wantErr bool
	}{
		{
			name:    "an import in a cluster that has the module",
			obj:     objectWith(),
			mode:    v1alpha2.UnifiedSnapshotterModeImport,
			present: true,
		},
		{
			name:    "an import in a cluster without the module",
			obj:     objectWith(),
			mode:    v1alpha2.UnifiedSnapshotterModeImport,
			wantErr: true,
		},
		{
			name:    "an import pinned to the built-in mechanism",
			obj:     objectWith(v1alpha2.AnnUseBuiltInSnapshotter),
			mode:    v1alpha2.UnifiedSnapshotterModeImport,
			present: true,
			wantErr: true,
		},
		{
			name:    "an import pinned to the unified mechanism",
			obj:     objectWith(v1alpha2.AnnUseUnifiedSnapshotter),
			mode:    v1alpha2.UnifiedSnapshotterModeImport,
			present: true,
		},
		// Everything below is a capture, which this rule has no say over: the built-in mechanism handles
		// captures, so neither the pin nor a missing module is its business.
		{
			name:    "a capture pinned to the built-in mechanism",
			obj:     objectWith(v1alpha2.AnnUseBuiltInSnapshotter),
			mode:    v1alpha2.UnifiedSnapshotterModeCapture,
			present: true,
		},
		{
			name: "a capture in a cluster without the module",
			obj:  objectWith(),
			mode: v1alpha2.UnifiedSnapshotterModeCapture,
		},
		{
			name: "an object created before spec.mode existed",
			obj:  objectWith(v1alpha2.AnnUseBuiltInSnapshotter),
			mode: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ImportModeRequiresUnifiedSnapshotter(tt.obj, tt.mode, tt.present)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
