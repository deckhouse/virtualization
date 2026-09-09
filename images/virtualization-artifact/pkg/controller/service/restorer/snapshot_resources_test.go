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

package restorer

import (
	"context"
	"encoding/json"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SnapshotResources.Prepare", func() {
	It("maps VirtualMachineMACAddressName by original network index", func() {
		vm := &v1alpha2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork, MAC: "02:00:00:00:00:11"},
				},
			},
		}

		vmmacs := []v1alpha2.VirtualMachineMACAddress{
			{
				TypeMeta: metav1.TypeMeta{
					Kind:       v1alpha2.VirtualMachineMACAddressKind,
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{Name: "vm-mac-secondary", Namespace: "default"},
				Status:     v1alpha2.VirtualMachineMACAddressStatus{Address: "02:00:00:00:00:11"},
			},
		}

		vmJSON, err := json.Marshal(vm)
		Expect(err).NotTo(HaveOccurred())

		vmmacsJSON, err := json.Marshal(vmmacs)
		Expect(err).NotTo(HaveOccurred())

		restorerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "restorer-secret", Namespace: "default"},
			Data: map[string][]byte{
				virtualMachineKey:             vmJSON,
				virtualMachineMACAddressesKey: vmmacsJSON,
			},
		}

		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"},
		}

		manifestReader := &secretManifestReader{restorer: NewSecretRestorer(fakeClient), secret: restorerSecret}
		resources := NewSnapshotResources(
			fakeClient, v1alpha2.VMOPTypeRestore, v1alpha2.SnapshotOperationModeStrict,
			manifestReader, vmSnapshot, "restore-uid",
		)
		Expect(resources.Prepare(context.Background())).To(Succeed())

		var restoredVM *v1alpha2.VirtualMachine
		for _, handler := range resources.GetObjectHandlers() {
			if obj, ok := handler.Object().(*v1alpha2.VirtualMachine); ok {
				restoredVM = obj
				break
			}
		}

		Expect(restoredVM).NotTo(BeNil())
		Expect(restoredVM.Spec.Networks[0].VirtualMachineMACAddressName).To(BeEmpty())
		Expect(restoredVM.Spec.Networks[1].VirtualMachineMACAddressName).To(Equal("vm-mac-secondary"))
	})
})

// manifests-download preserves status verbatim, but the sibling manifests-with-data-restoration
// endpoint strips it (transform.SanitizeForRestore), so a status-less VirtualMachine manifest is a
// reachable shape. It must not index past the MAC order derived from that status.
var _ = Describe("SnapshotResources.Prepare with a status-less captured VirtualMachine", func() {
	It("fails with a diagnosable error instead of panicking", func() {
		vm := &v1alpha2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork},
				},
			},
			// Status deliberately absent.
		}

		vmmacs := []v1alpha2.VirtualMachineMACAddress{{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineMACAddressKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{Name: "vm-mac-secondary", Namespace: "default"},
			Status:     v1alpha2.VirtualMachineMACAddressStatus{Address: "02:00:00:00:00:11"},
		}}

		vmJSON, err := json.Marshal(vm)
		Expect(err).NotTo(HaveOccurred())
		vmmacsJSON, err := json.Marshal(vmmacs)
		Expect(err).NotTo(HaveOccurred())

		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		manifestReader := &secretManifestReader{
			restorer: NewSecretRestorer(fakeClient),
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "restorer-secret", Namespace: "default"},
				Data: map[string][]byte{
					virtualMachineKey:             vmJSON,
					virtualMachineMACAddressesKey: vmmacsJSON,
				},
			},
		}

		resources := NewSnapshotResources(
			fakeClient, v1alpha2.VMOPTypeRestore, v1alpha2.SnapshotOperationModeStrict,
			manifestReader,
			&v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
				Spec:       v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "vm"},
			},
			"restore-uid",
		)

		Expect(resources.Prepare(context.Background())).To(MatchError(ContainSubstring("cannot restore the MAC address order")))
	})
})

var _ = Describe("virtualDiskSnapshotNames", func() {
	It("reads Status.VirtualDiskSnapshotNames for a built-in (non-annotated) snapshot", func() {
		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				VirtualDiskSnapshotNames: []string{"vds1", "vds2"},
				// A ChildrenSnapshotRefs entry here must be ignored for a legacy-captured snapshot.
				ChildrenSnapshotRefs: []v1alpha2.UnifiedSnapshotterChildRef{
					{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskSnapshotKind, Name: "should-be-ignored"},
				},
			},
		}
		Expect(virtualDiskSnapshotNames(vmSnapshot)).To(Equal([]string{"vds1", "vds2"}))
	})

	It("filters Status.ChildrenSnapshotRefs by apiVersion and kind for a unified-snapshotter captured snapshot", func() {
		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{},
				// Must be ignored for a unified-captured snapshot, even though it's non-empty.
				VirtualDiskSnapshotNames: []string{"should-be-ignored"},
				ChildrenSnapshotRefs: []v1alpha2.UnifiedSnapshotterChildRef{
					{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskSnapshotKind, Name: "vds1"},
					{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: "SomeOtherKind", Name: "not-a-disk-snapshot"},
					// Same kind, foreign group: not one of ours.
					{APIVersion: "internal.virtualization.deckhouse.io/v1beta1", Kind: v1alpha2.VirtualDiskSnapshotKind, Name: "not-ours"},
					{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskSnapshotKind, Name: "vds2"},
				},
			},
		}
		Expect(virtualDiskSnapshotNames(vmSnapshot)).To(Equal([]string{"vds1", "vds2"}))
	})

	It("returns an empty slice for a unified-snapshotter snapshot with no disk children", func() {
		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{},
			},
		}
		Expect(virtualDiskSnapshotNames(vmSnapshot)).To(BeEmpty())
	})
})

var _ = Describe("AddOriginalMetadata", func() {
	It("waits instead of losing metadata when the captured disk's content is not bound yet", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: "default"}}
		vdSnapshot := &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot", Namespace: "default"},
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				CaptureState: &v1alpha2.UnifiedSnapshotterCaptureState{},
			},
		}

		Expect(AddOriginalMetadata(context.Background(), vd, vdSnapshot, fakeClient)).To(MatchError(common.ErrQueueing))
	})

	It("does not silence a built-in VirtualDiskSnapshot that is missing its VolumeSnapshotName", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: "default"}}
		vdSnapshot := &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot", Namespace: "default"},
		}

		Expect(AddOriginalMetadata(context.Background(), vd, vdSnapshot, fakeClient)).NotTo(Succeed())
	})

	It("still copies original annotations/labels when VolumeSnapshotName is set (built-in mechanism)", func() {
		vs := &vsv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vs1", Namespace: "default",
				Annotations: map[string]string{
					annotations.AnnVirtualDiskOriginalAnnotations: `{"custom-key":"custom-value"}`,
				},
			},
		}
		// testutil.NewFakeClientWithObjects's scheme doesn't register the external-snapshotter
		// VolumeSnapshot type, so this one test (needing a real VolumeSnapshot) builds its own.
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()

		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: "default"}}
		vdSnapshot := &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot", Namespace: "default"},
			Status:     v1alpha2.VirtualDiskSnapshotStatus{VolumeSnapshotName: "vs1"},
		}

		Expect(AddOriginalMetadata(context.Background(), vd, vdSnapshot, fakeClient)).To(Succeed())
		Expect(vd.Annotations).To(HaveKeyWithValue("custom-key", "custom-value"))
	})
})

var _ = Describe("addOriginalMetadataFromCapturedDisk", func() {
	captured := func() *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{
			Name:        "vd",
			Namespace:   "default",
			Annotations: map[string]string{"custom-key": "captured-value", "other": "kept"},
			Labels:      map[string]string{"env": "captured"},
		}}
	}

	It("copies annotations and labels onto a bare restored disk", func() {
		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: "default"}}
		addOriginalMetadataFromCapturedDisk(vd, captured())
		Expect(vd.Annotations).To(HaveKeyWithValue("custom-key", "captured-value"))
		Expect(vd.Annotations).To(HaveKeyWithValue("other", "kept"))
		Expect(vd.Labels).To(HaveKeyWithValue("env", "captured"))
	})

	It("does not overwrite what the restore already set", func() {
		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{
			Name:        "vd",
			Namespace:   "default",
			Annotations: map[string]string{"custom-key": "restore-value"},
			Labels:      map[string]string{"env": "restore"},
		}}
		addOriginalMetadataFromCapturedDisk(vd, captured())
		Expect(vd.Annotations).To(HaveKeyWithValue("custom-key", "restore-value"))
		Expect(vd.Annotations).To(HaveKeyWithValue("other", "kept"))
		Expect(vd.Labels).To(HaveKeyWithValue("env", "restore"))
	})

	It("tolerates a nil captured disk and a disk with no metadata", func() {
		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: "default"}}
		addOriginalMetadataFromCapturedDisk(vd, nil)
		addOriginalMetadataFromCapturedDisk(vd, &v1alpha2.VirtualDisk{})
		Expect(vd.Annotations).To(BeEmpty())
		Expect(vd.Labels).To(BeEmpty())
	})
})

var _ = Describe("hasVirtualMachineOwner", func() {
	withOwner := func(refs ...metav1.OwnerReference) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd", OwnerReferences: refs}}
	}

	It("reports a VirtualMachine owner", func() {
		Expect(hasVirtualMachineOwner(withOwner(metav1.OwnerReference{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineKind,
			Name:       "vm",
		}))).To(BeTrue())
	})

	It("ignores owners of other kinds", func() {
		Expect(hasVirtualMachineOwner(withOwner(metav1.OwnerReference{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachinePoolKind,
			Name:       "pool",
		}))).To(BeFalse())
	})

	It("reports no owner for an unowned or absent disk", func() {
		Expect(hasVirtualMachineOwner(withOwner())).To(BeFalse())
		Expect(hasVirtualMachineOwner(nil)).To(BeFalse())
	})
})
