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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// An import never enters the capture state machine, so status.captureState stays nil on it and only
// spec.mode says the content came from the core. Both of these read that through IsUnifiedDiskCapture;
// reading the raw field instead sent an imported snapshot down the built-in path, looking for a CSI
// VolumeSnapshot that an import never creates.
var _ = Describe("an imported VirtualDiskSnapshot on the restore path", func() {
	importedSnapshot := func() *v1alpha2.VirtualDiskSnapshot {
		return &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vdsnapshot", Namespace: "default"},
			Spec:       v1alpha2.VirtualDiskSnapshotSpec{Mode: v1alpha2.UnifiedSnapshotterModeImport},
			// captureState deliberately nil, VolumeSnapshotName deliberately empty: that is the whole
			// shape an import has.
		}
	}

	It("is not mistaken for a built-in capture by AddOriginalMetadata", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: "default"}}

		// Not yet bound, so this waits — the point is that it does NOT report a missing VolumeSnapshot.
		err = AddOriginalMetadata(context.Background(), vd, importedSnapshot(), fakeClient)
		Expect(err).To(MatchError(common.ErrQueueing))
		Expect(err).NotTo(MatchError(ContainSubstring("volume snapshot")))
	})

	It("is not mistaken for a built-in capture by virtualDiskHadOwnerReference", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		r := &SnapshotResources{client: fakeClient}

		// The built-in branch would answer "no owner reference" from an empty VolumeSnapshotName and
		// silently drop the VirtualMachine ownerRef the restored disk is supposed to get back. The
		// unified branch has to wait for the content instead.
		_, err = r.virtualDiskHadOwnerReference(context.Background(), "default", importedSnapshot())
		Expect(err).To(MatchError(common.ErrQueueing))
	})
})

// A restored disk has to come back under the name it was captured as, and for an imported snapshot the
// captured manifest is the only record of it: spec carries no source (an import is forbidden to name
// one) and the core publishes no status.sourceRef either. Read from the spec alone the disk was built
// nameless, and the API server rejected it with "metadata.name: Required value".
var _ = Describe("naming a restored VirtualDisk", func() {
	It("takes the name from the captured manifest when the snapshot names no source", func() {
		vd := &v1alpha2.VirtualDisk{}
		captured := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "vd-root",
				Annotations: map[string]string{"custom-key": "custom-value"},
			},
		}

		addOriginalMetadataFromCapturedDisk(vd, captured)

		Expect(vd.Name).To(Equal("vd-root"))
		Expect(vd.Annotations).To(HaveKeyWithValue("custom-key", "custom-value"))
	})

	It("leaves a name the spec already resolved alone", func() {
		vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "from-spec"}}
		captured := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "from-manifest"}}

		addOriginalMetadataFromCapturedDisk(vd, captured)

		Expect(vd.Name).To(Equal("from-spec"))
	})
})

// A captured manifest carries the metadata.namespace it had when it was taken. For a snapshot captured
// in place that is also where it is restored, so nothing ever noticed; an imported snapshot breaks the
// coincidence, and a restore then recreated the VirtualMachine in the namespace the original came from
// instead of the one the operation was asked for.
var _ = Describe("confining a restore to the snapshot's namespace", func() {
	const (
		restoreInto  = "target-ns"
		capturedFrom = "source-ns"
	)

	newResources := func() SnapshotResources {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: restoreInto},
		}
		return NewSnapshotResources(fakeClient, v1alpha2.VMOPTypeRestore, v1alpha2.SnapshotOperationModeStrict, nil, vmSnapshot, "uuid")
	}

	foreign := func() metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: "obj", Namespace: capturedFrom}
	}

	It("moves every captured object into the namespace being restored into", func() {
		r := newResources()

		vm := &v1alpha2.VirtualMachine{ObjectMeta: foreign()}
		vmip := &v1alpha2.VirtualMachineIPAddress{ObjectMeta: foreign()}
		secret := &corev1.Secret{ObjectMeta: foreign()}
		vd := &v1alpha2.VirtualDisk{ObjectMeta: foreign()}
		vmmac := &v1alpha2.VirtualMachineMACAddress{ObjectMeta: foreign()}
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{ObjectMeta: foreign()}

		r.confineToSnapshotNamespace(vm, vmip, secret, vd, vmmac, vmbda)

		for _, obj := range []client.Object{vm, vmip, secret, vd, vmmac, vmbda} {
			Expect(obj.GetNamespace()).To(Equal(restoreInto), "%T stayed in %q", obj, obj.GetNamespace())
		}
	})

	// A resource the snapshot did not capture arrives as a typed nil, which must not panic the restore.
	It("ignores what was not captured", func() {
		r := newResources()

		var absentSecret *corev1.Secret
		var absentIP *v1alpha2.VirtualMachineIPAddress

		Expect(func() { r.confineToSnapshotNamespace(absentSecret, absentIP) }).NotTo(Panic())
	})

	// The pinning happens where each object is read, so a resource added later could miss it. This is
	// what stops that from becoming a silent write into somebody else's namespace.
	It("refuses an object that escaped the pinning", func() {
		r := newResources()

		escaped := &v1alpha2.VirtualMachine{ObjectMeta: foreign()}
		Expect(r.assertConfinedToSnapshotNamespace(escaped)).To(MatchError(ContainSubstring("restores only into its own namespace")))

		pinned := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "obj", Namespace: restoreInto}}
		Expect(r.assertConfinedToSnapshotNamespace(pinned)).To(Succeed())
	})
})

// fakeManifestReader returns objects still carrying the namespace they were captured from, the way an
// imported snapshot's manifests do.
type fakeManifestReader struct {
	namespace string
}

func (f fakeManifestReader) RestoreVirtualMachine(context.Context) (*v1alpha2.VirtualMachine, error) {
	return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: f.namespace}}, nil
}

func (f fakeManifestReader) RestoreProvisioner(context.Context) (*corev1.Secret, error) {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "provisioner", Namespace: f.namespace}}, nil
}

func (f fakeManifestReader) RestoreVirtualMachineIPAddress(context.Context) (*v1alpha2.VirtualMachineIPAddress, error) {
	return &v1alpha2.VirtualMachineIPAddress{ObjectMeta: metav1.ObjectMeta{Name: "vmip", Namespace: f.namespace}}, nil
}

func (f fakeManifestReader) RestoreVirtualMachineMACAddresses(context.Context) ([]*v1alpha2.VirtualMachineMACAddress, error) {
	return nil, nil
}

func (f fakeManifestReader) RestoreMACAddressOrder(context.Context) ([]string, error) {
	return nil, nil
}

func (f fakeManifestReader) RestoreVirtualMachineBlockDeviceAttachments(context.Context) ([]*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	return []*v1alpha2.VirtualMachineBlockDeviceAttachment{
		{ObjectMeta: metav1.ObjectMeta{Name: "vmbda", Namespace: f.namespace}},
	}, nil
}

// The end-to-end shape of the bug: a restore prepared from manifests captured elsewhere must still
// create everything in the namespace of the snapshot it was asked to restore.
var _ = Describe("preparing a restore from manifests captured in another namespace", func() {
	It("leaves nothing pointing at the namespace they were captured from", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: "target-ns"},
			Spec:       v1alpha2.VirtualMachineSnapshotSpec{Mode: v1alpha2.UnifiedSnapshotterModeImport},
		}
		r := NewSnapshotResources(
			fakeClient,
			v1alpha2.VMOPTypeRestore,
			v1alpha2.SnapshotOperationModeStrict,
			fakeManifestReader{namespace: "source-ns"},
			vmSnapshot,
			"uuid",
		)

		Expect(r.Prepare(context.Background())).To(Succeed())
		Expect(r.GetObjectHandlers()).NotTo(BeEmpty())

		for _, handler := range r.GetObjectHandlers() {
			obj := handler.Object()
			Expect(obj.GetNamespace()).To(Equal("target-ns"),
				"%T %q would be created in %q", obj, obj.GetName(), obj.GetNamespace())
		}
	})
})
