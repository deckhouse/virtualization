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

package watcher

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestEnqueueRequestsFromVDs(t *testing.T) {
	cvi := func(name, diskName, diskNamespace string) *v1alpha2.ClusterVirtualImage {
		return &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: v1alpha2.ClusterVirtualImageSpec{DataSource: v1alpha2.ClusterVirtualImageDataSource{
				Type: v1alpha2.DataSourceTypeObjectRef,
				ObjectRef: &v1alpha2.ClusterVirtualImageObjectRef{
					Kind:      v1alpha2.VirtualDiskKind,
					Name:      diskName,
					Namespace: diskNamespace,
				},
			}},
		}
	}

	c, err := testutil.NewFakeClientWithObjects(
		cvi("from-disk", "disk", "ns-a"),
		cvi("from-namesake-disk", "disk", "ns-b"),
		cvi("from-neighbour-disk", "other", "ns-a"),
	)
	if err != nil {
		t.Fatal(err)
	}

	vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "disk", Namespace: "ns-a"}}
	requests := NewVirtualDiskWatcher(c).enqueueRequestsFromVDs(context.Background(), vd)

	if len(requests) != 1 || requests[0].Name != "from-disk" {
		t.Errorf("enqueueRequestsFromVDs() = %v, want only the image created from the disk", requests)
	}
}
