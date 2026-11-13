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

package accessclaim

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newCoreClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestParentRefMatchesOnlyAVirtualMachineSnapshotOfTheModule(t *testing.T) {
	child := &v1alpha2.VirtualDiskSnapshot{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{
		{APIVersion: "other.io/v1", Kind: v1alpha2.VirtualMachineSnapshotKind, Name: "foreign"},
		{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineSnapshotKind, Name: "vms"},
	}}}
	if ref := ParentRef(child); ref == nil || ref.Name != "vms" {
		t.Fatalf("ParentRef = %+v, want the VirtualMachineSnapshot of the module", ref)
	}

	standalone := &v1alpha2.VirtualDiskSnapshot{}
	if ref := ParentRef(standalone); ref != nil {
		t.Fatalf("ParentRef = %+v for a snapshot taken on its own", ref)
	}
}

func TestChildConsistencyFollowsTheParent(t *testing.T) {
	parentRef := &metav1.OwnerReference{Name: "vms", UID: "vms-uid"}
	child := &v1alpha2.VirtualDiskSnapshot{ObjectMeta: metav1.ObjectMeta{Namespace: "ns"}}

	for name, tc := range map[string]struct {
		objs                     []client.Object
		wantConsistent, wantDone bool
	}{
		"parent in progress": {
			objs: []client.Object{&v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: "ns", UID: "vms-uid"},
				Status:     v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhaseInProgress},
			}},
		},
		"parent ready and consistent": {
			objs: []client.Object{&v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: "ns", UID: "vms-uid"},
				Status:     v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhaseReady, Consistent: ptr.To(true)},
			}},
			wantConsistent: true, wantDone: true,
		},
		"parent failed": {
			objs: []client.Object{&v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: "ns", UID: "vms-uid"},
				Status:     v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhaseFailed, Consistent: ptr.To(false)},
			}},
			wantDone: true,
		},
		"parent gone": {
			wantDone: true,
		},
		"parent replaced under the same name": {
			objs: []client.Object{&v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: "ns", UID: "other-uid"},
				Status:     v1alpha2.VirtualMachineSnapshotStatus{Phase: v1alpha2.VirtualMachineSnapshotPhaseReady, Consistent: ptr.To(true)},
			}},
			wantDone: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			consistent, settled, err := ChildConsistency(context.Background(), newCoreClient(t, tc.objs...), child, parentRef)
			if err != nil {
				t.Fatal(err)
			}

			if consistent != tc.wantConsistent || settled != tc.wantDone {
				t.Errorf("ChildConsistency = (%v, %v), want (%v, %v)", consistent, settled, tc.wantConsistent, tc.wantDone)
			}
		})
	}
}
