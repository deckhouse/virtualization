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

package snapshotter

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestSnapshotter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Snapshotter Suite")
}

func objectWith(annotations ...string) metav1.Object {
	meta := &metav1.ObjectMeta{}
	if len(annotations) > 0 {
		meta.Annotations = map[string]string{}
		for _, a := range annotations {
			meta.Annotations[a] = ""
		}
	}
	return meta
}

var _ = Describe("UseUnified", func() {
	Context("with neither annotation", func() {
		It("follows the cluster default", func() {
			Expect(UseUnified(objectWith(), true)).To(BeTrue())
			Expect(UseUnified(objectWith(), false)).To(BeFalse())
		})

		It("follows the default for an object with unrelated annotations", func() {
			other := &metav1.ObjectMeta{Annotations: map[string]string{"example.com/other": "1"}}
			Expect(UseUnified(other, true)).To(BeTrue())
			Expect(UseUnified(other, false)).To(BeFalse())
		})
	})

	It("pins to the unified mechanism regardless of the default", func() {
		obj := objectWith(v1alpha2.AnnUseUnifiedSnapshotter)
		Expect(UseUnified(obj, true)).To(BeTrue())
		Expect(UseUnified(obj, false)).To(BeTrue())
	})

	It("pins to the built-in mechanism regardless of the default", func() {
		obj := objectWith(v1alpha2.AnnUseBuiltInSnapshotter)
		Expect(UseUnified(obj, true)).To(BeFalse())
		Expect(UseUnified(obj, false)).To(BeFalse())
	})

	// Admission rejects this combination, so the only requirement is that both controllers agree on the
	// answer and neither leaves the object unowned.
	It("prefers the built-in mechanism when an invalid object pins both", func() {
		obj := objectWith(v1alpha2.AnnUseUnifiedSnapshotter, v1alpha2.AnnUseBuiltInSnapshotter)
		Expect(UseUnified(obj, true)).To(BeFalse())
		Expect(UseUnified(obj, false)).To(BeFalse())
	})

	It("gives the value of the annotation no meaning", func() {
		obj := &metav1.ObjectMeta{Annotations: map[string]string{v1alpha2.AnnUseBuiltInSnapshotter: "false"}}
		Expect(UseUnified(obj, true)).To(BeFalse())
	})
})

// The upgrade case: turning the unified mechanism on must not reassign snapshots that the built-in one
// already took, or the unified VirtualMachineSnapshot controller would re-plan a finished capture.
var _ = Describe("routing an already-captured object", func() {
	It("keeps a built-in VirtualMachineSnapshot even once unified is the default", func() {
		vms := &v1alpha2.VirtualMachineSnapshot{
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				Phase:                            v1alpha2.VirtualMachineSnapshotPhaseReady,
				VirtualMachineSnapshotSecretName: "vms-secret",
			},
		}
		Expect(UseUnifiedForVirtualMachineSnapshot(vms, true)).To(BeFalse())
	})

	It("keeps a unified VirtualMachineSnapshot even once unified is no longer the default", func() {
		vms := &v1alpha2.VirtualMachineSnapshot{
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				Phase:        v1alpha2.VirtualMachineSnapshotPhaseReady,
				CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{},
			},
		}
		Expect(UseUnifiedForVirtualMachineSnapshot(vms, false)).To(BeTrue())
	})

	It("keeps a built-in VirtualDiskSnapshot even once unified is the default", func() {
		vds := &v1alpha2.VirtualDiskSnapshot{
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
				VolumeSnapshotName: "vs-1",
			},
		}
		Expect(routeDisk(vds, true)).To(BeFalse())
	})

	It("keeps a unified VirtualDiskSnapshot even once unified is no longer the default", func() {
		vds := &v1alpha2.VirtualDiskSnapshot{
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				Phase:        v1alpha2.VirtualDiskSnapshotPhaseReady,
				CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{},
			},
		}
		Expect(routeDisk(vds, false)).To(BeTrue())
	})

	It("follows the default while nothing has captured the object yet", func() {
		Expect(UseUnifiedForVirtualMachineSnapshot(&v1alpha2.VirtualMachineSnapshot{}, true)).To(BeTrue())
		Expect(UseUnifiedForVirtualMachineSnapshot(&v1alpha2.VirtualMachineSnapshot{}, false)).To(BeFalse())
		Expect(routeDisk(&v1alpha2.VirtualDiskSnapshot{}, true)).To(BeTrue())
		Expect(routeDisk(&v1alpha2.VirtualDiskSnapshot{}, false)).To(BeFalse())
	})

	It("lets an explicit pin override the recorded capture", func() {
		vms := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""},
			},
			Status: v1alpha2.VirtualMachineSnapshotStatus{VirtualMachineSnapshotSecretName: "vms-secret"},
		}
		Expect(UseUnifiedForVirtualMachineSnapshot(vms, false)).To(BeTrue())
	})
})

func newReader(objs ...client.Object) client.Reader {
	scheme := apiruntime.NewScheme()
	Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

// routeDisk routes a standalone VirtualDiskSnapshot: no parent to read, so the reader is never used.
func routeDisk(vds *v1alpha2.VirtualDiskSnapshot, unifiedPresent bool) bool {
	useUnified, err := UseUnifiedForVirtualDiskSnapshot(context.Background(), nil, vds, unifiedPresent)
	Expect(err).NotTo(HaveOccurred())
	return useUnified
}

func childOf(parentName string) *v1alpha2.VirtualDiskSnapshot {
	return &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vds1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineSnapshotKind,
				Name:       parentName,
			}},
		},
	}
}

func parent(name string, captureState *v1alpha2.UnifiedSnapshotterCaptureState) *v1alpha2.VirtualMachineSnapshot {
	return &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns1"},
		Status:     v1alpha2.VirtualMachineSnapshotStatus{CaptureState: captureState},
	}
}

// A child must never split from its parent: the two would take the same disk through both mechanisms
// at once. It derives the mechanism from the parent's status, so an upgrade that moves the cluster
// default mid-capture cannot separate them.
var _ = DescribeTable("routing a child VirtualDiskSnapshot",
	func(captureState *v1alpha2.UnifiedSnapshotterCaptureState, unifiedPresent, expected bool) {
		reader := newReader(parent("vms1", captureState))

		useUnified, err := UseUnifiedForVirtualDiskSnapshot(context.Background(), reader, childOf("vms1"), unifiedPresent)
		Expect(err).NotTo(HaveOccurred())
		Expect(useUnified).To(Equal(expected))
	},
	Entry("follows a unified parent against the default",
		&v1alpha2.UnifiedSnapshotterCaptureState{}, false, true),
	Entry("follows a unified parent with the default",
		&v1alpha2.UnifiedSnapshotterCaptureState{}, true, true),
	Entry("follows a built-in parent against the default",
		nil, true, false),
	Entry("follows a built-in parent with the default",
		nil, false, false),
)

var _ = Describe("routing a child whose parent is gone", func() {
	It("falls back to the cluster default", func() {
		reader := newReader()

		for _, unifiedPresent := range []bool{true, false} {
			useUnified, err := UseUnifiedForVirtualDiskSnapshot(context.Background(), reader, childOf("missing"), unifiedPresent)
			Expect(err).NotTo(HaveOccurred())
			Expect(useUnified).To(Equal(unifiedPresent))
		}
	})

	It("prefers its own recorded capture over the parent lookup", func() {
		// A built-in parent that would otherwise pull the child the other way.
		reader := newReader(parent("vms1", nil))

		child := childOf("vms1")
		child.Status.CaptureState = &v1alpha2.UnifiedSnapshotterCaptureState{}

		useUnified, err := UseUnifiedForVirtualDiskSnapshot(context.Background(), reader, child, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(useUnified).To(BeTrue())
	})
})
