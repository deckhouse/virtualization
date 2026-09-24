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

package object

import (
	"errors"
	"fmt"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsGone(t *testing.T) {
	snapshots := schema.GroupResource{
		Group:    "virtualization.deckhouse.io",
		Resource: "virtualmachinesnapshots",
	}
	disks := schema.GroupResource{
		Group:    "virtualization.deckhouse.io",
		Resource: "virtualdisks",
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "the object being reconciled is gone",
			err:  k8serrors.NewNotFound(snapshots, "vmsnapshot"),
			want: true,
		},
		{
			// Every write after the first read comes back wrapped by the call that made it.
			name: "and stays recognisable through wrapping",
			err:  fmt.Errorf("patch status: %w", k8serrors.NewNotFound(snapshots, "vmsnapshot")),
			want: true,
		},
		{
			// A disk or machine a reconcile waits for is absent all the time, and the code that waits
			// for it must keep seeing that.
			name: "another kind of object is absent",
			err:  k8serrors.NewNotFound(disks, "vd-root"),
		},
		{
			name: "another object of the same kind is absent",
			err:  k8serrors.NewNotFound(snapshots, "some-other-snapshot"),
		},
		{
			name: "a conflict is not an absence",
			err:  k8serrors.NewConflict(snapshots, "vmsnapshot", errors.New("modified")),
		},
		{
			name: "a plain error carries no details to match",
			err:  errors.New("virtualmachinesnapshots.virtualization.deckhouse.io \"vmsnapshot\" not found"),
		},
		{
			name: "no error at all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGone(tt.err, snapshots, "vmsnapshot"); got != tt.want {
				t.Errorf("IsGone(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
