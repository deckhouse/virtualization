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

package rest

import (
	"fmt"
	"net/http"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/restore"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestAPIError_StatusCodes(t *testing.T) {
	gr := schema.GroupResource{Group: "virtualization.deckhouse.io", Resource: v1alpha2.VirtualMachineSnapshotResource}
	r := NewManifestsWithDataRestorationREST(v1alpha2.VirtualMachineSnapshotResource, nil)

	tests := []struct {
		name string
		err  error
		want int32
	}{
		{
			name: "not-ready node is retryable",
			err:  fmt.Errorf("VirtualMachineSnapshot ns/vms: %w", restore.ErrSnapshotNotReady),
			want: http.StatusConflict,
		},
		{
			name: "wrapped NotFound keeps its 404",
			err:  fmt.Errorf("get VirtualMachineSnapshot ns/vms: %w", k8serrors.NewNotFound(gr, "vms")),
			want: http.StatusNotFound,
		},
		{
			name: "anything else is an internal error",
			err:  fmt.Errorf("boom"),
			want: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.apiError("vms", tt.err)
			var statusErr k8serrors.APIStatus
			if !errorAs(got, &statusErr) {
				t.Fatalf("apiError returned %T, want an APIStatus error", got)
			}
			if code := statusErr.Status().Code; code != tt.want {
				t.Errorf("code = %d, want %d (message: %s)", code, tt.want, statusErr.Status().Message)
			}
		})
	}
}

func errorAs(err error, target *k8serrors.APIStatus) bool {
	s, ok := err.(k8serrors.APIStatus)
	if ok {
		*target = s
	}
	return ok
}

func TestRejectForeignTargetNamespace(t *testing.T) {
	tests := []struct {
		name      string
		requested []string
		wantErr   bool
	}{
		{name: "absent", requested: nil},
		{name: "empty value", requested: []string{""}},
		{name: "the snapshot's own namespace", requested: []string{"ns"}},
		{name: "a foreign namespace", requested: []string{"other"}, wantErr: true},
		// A repeated key must not let a foreign namespace ride along behind an acceptable one: the
		// query decoder keeps every value precisely so this case is visible here.
		{name: "foreign namespace behind an acceptable one", requested: []string{"ns", "other"}, wantErr: true},
		{name: "foreign namespace behind an empty one", requested: []string{"", "other"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectForeignTargetNamespace(tt.requested, "ns")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
