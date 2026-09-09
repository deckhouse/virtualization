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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ssstoragev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// fakeManifestDownloader is a manifestDownloader stub returning a fixed manifest set, so
// unifiedManifestReader's kind-demux logic can be tested without a real aggregated-APIService call.
type fakeManifestDownloader struct {
	manifests []RawManifest
	err       error
}

func (f *fakeManifestDownloader) DownloadManifests(_ context.Context, _ string) ([]RawManifest, error) {
	return f.manifests, f.err
}

func rawManifest(apiVersion, kind string, obj any) RawManifest {
	data, err := json.Marshal(obj)
	Expect(err).NotTo(HaveOccurred())
	return RawManifest{TypeMeta: metav1.TypeMeta{APIVersion: apiVersion, Kind: kind}, Raw: data}
}

var _ = Describe("unifiedManifestReader", func() {
	It("demuxes a mixed manifest set by apiVersion and kind", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm"},
			Spec: v1alpha2.VirtualMachineSpec{
				Provisioning: &v1alpha2.Provisioning{
					Type: v1alpha2.ProvisioningTypeUserDataRef,
					UserDataRef: &v1alpha2.UserDataRef{
						Kind: v1alpha2.UserDataRefKindSecret,
						Name: "cloud-init",
					},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork, MAC: "02:00:00:00:00:11"},
				},
			},
		}
		vmip := &v1alpha2.VirtualMachineIPAddress{ObjectMeta: metav1.ObjectMeta{Name: "vmip"}}
		vmmac1 := &v1alpha2.VirtualMachineMACAddress{ObjectMeta: metav1.ObjectMeta{Name: "mac1"}}
		vmmac2 := &v1alpha2.VirtualMachineMACAddress{ObjectMeta: metav1.ObjectMeta{Name: "mac2"}}
		provisioner := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cloud-init"}}
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{ObjectMeta: metav1.ObjectMeta{Name: "vmbda"}}

		downloader := &fakeManifestDownloader{manifests: []RawManifest{
			rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineKind, vm),
			rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineIPAddressKind, vmip),
			rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineMACAddressKind, vmmac1),
			rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineMACAddressKind, vmmac2),
			rawManifest("v1", "Secret", provisioner),
			rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineBlockDeviceAttachmentKind, vmbda),
			// Same Kind, foreign group (KubeVirt's own VirtualMachine): must not be mistaken for ours.
			rawManifest("internal.virtualization.deckhouse.io/v1", v1alpha2.VirtualMachineKind,
				&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "kubevirt-vm"}}),
		}}
		reader := &unifiedManifestReader{client: downloader, contentName: "content-1"}
		ctx := context.Background()

		gotVM, err := reader.RestoreVirtualMachine(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(gotVM.Name).To(Equal("vm"))

		gotVMIP, err := reader.RestoreVirtualMachineIPAddress(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(gotVMIP.Name).To(Equal("vmip"))

		gotVMMACs, err := reader.RestoreVirtualMachineMACAddresses(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(gotVMMACs).To(HaveLen(2))

		gotProvisioner, err := reader.RestoreProvisioner(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(gotProvisioner.Name).To(Equal("cloud-init"))

		gotVMBDAs, err := reader.RestoreVirtualMachineBlockDeviceAttachments(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(gotVMBDAs).To(HaveLen(1))
		Expect(gotVMBDAs[0].Name).To(Equal("vmbda"))

		order, err := reader.RestoreMACAddressOrder(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(order).To(Equal([]string{"", "02:00:00:00:00:11"}))
	})

	It("returns nil, not an error, for optional resources that weren't captured", func() {
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm"}}
		downloader := &fakeManifestDownloader{manifests: []RawManifest{
			rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineKind, vm),
		}}
		reader := &unifiedManifestReader{client: downloader, contentName: "content-1"}
		ctx := context.Background()

		vmip, err := reader.RestoreVirtualMachineIPAddress(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(vmip).To(BeNil())

		provisioner, err := reader.RestoreProvisioner(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(provisioner).To(BeNil())

		vmmacs, err := reader.RestoreVirtualMachineMACAddresses(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(vmmacs).To(BeEmpty())

		vmbdas, err := reader.RestoreVirtualMachineBlockDeviceAttachments(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(vmbdas).To(BeEmpty())
	})

	It("errors when the mandatory VirtualMachine manifest is missing", func() {
		downloader := &fakeManifestDownloader{manifests: nil}
		reader := &unifiedManifestReader{client: downloader, contentName: "content-1"}

		_, err := reader.RestoreVirtualMachine(context.Background())
		Expect(err).To(HaveOccurred())
	})

	It("downloads at most once per reader across multiple calls", func() {
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm"}}
		downloader := &countingManifestDownloader{manifests: []RawManifest{
			rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineKind, vm),
		}}
		reader := &unifiedManifestReader{client: downloader, contentName: "content-1"}
		ctx := context.Background()

		_, err := reader.RestoreVirtualMachine(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = reader.RestoreVirtualMachineIPAddress(ctx)
		Expect(err).NotTo(HaveOccurred())
		_, err = reader.RestoreMACAddressOrder(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(downloader.calls).To(Equal(1))
	})
})

type countingManifestDownloader struct {
	manifests []RawManifest
	calls     int
}

func (c *countingManifestDownloader) DownloadManifests(_ context.Context, _ string) ([]RawManifest, error) {
	c.calls++
	return c.manifests, nil
}

var _ = Describe("unifiedManifestReader keepIPAddress", func() {
	newReader := func(keep v1alpha2.KeepIPAddress, vmip *v1alpha2.VirtualMachineIPAddress) *unifiedManifestReader {
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm"}}
		return &unifiedManifestReader{
			client: &fakeManifestDownloader{manifests: []RawManifest{
				rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineKind, vm),
				rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineIPAddressKind, vmip),
			}},
			contentName:   "content-1",
			keepIPAddress: keep,
		}
	}

	autoVMIP := func() *v1alpha2.VirtualMachineIPAddress {
		return &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "vmip"},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: "10.66.10.5"},
		}
	}

	// Parity with the built-in mechanism, which performs this conversion at capture time
	// (SecretRestorer.setVirtualMachineIPAddress). The unified capture stores manifests verbatim.
	It("pins an Auto address to Static with keepIPAddress=Always", func() {
		vmip, err := newReader(v1alpha2.KeepIPAddressAlways, autoVMIP()).RestoreVirtualMachineIPAddress(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(vmip.Spec.Type).To(Equal(v1alpha2.VirtualMachineIPAddressTypeStatic))
		Expect(vmip.Spec.StaticIP).To(Equal("10.66.10.5"))
	})

	It("leaves an Auto address alone with keepIPAddress=Never", func() {
		vmip, err := newReader(v1alpha2.KeepIPAddressNever, autoVMIP()).RestoreVirtualMachineIPAddress(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(vmip.Spec.Type).To(Equal(v1alpha2.VirtualMachineIPAddressTypeAuto))
		Expect(vmip.Spec.StaticIP).To(BeEmpty())
	})

	It("leaves an already Static address alone with keepIPAddress=Always", func() {
		static := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "vmip"},
			Spec: v1alpha2.VirtualMachineIPAddressSpec{
				Type:     v1alpha2.VirtualMachineIPAddressTypeStatic,
				StaticIP: "10.66.10.9",
			},
			Status: v1alpha2.VirtualMachineIPAddressStatus{Address: "10.66.10.9"},
		}
		vmip, err := newReader(v1alpha2.KeepIPAddressAlways, static).RestoreVirtualMachineIPAddress(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(vmip.Spec.StaticIP).To(Equal("10.66.10.9"))
	})

	It("fails loudly rather than silently allocating a new address", func() {
		unallocated := autoVMIP()
		unallocated.Status.Address = ""
		_, err := newReader(v1alpha2.KeepIPAddressAlways, unallocated).RestoreVirtualMachineIPAddress(context.Background())
		Expect(err).To(MatchError(ContainSubstring("keepIPAddress=Always")))
	})
})

var _ = Describe("unifiedManifestReader provisioner", func() {
	vmWithUserDataRef := func(name string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm"},
			Spec: v1alpha2.VirtualMachineSpec{
				Provisioning: &v1alpha2.Provisioning{
					Type:        v1alpha2.ProvisioningTypeUserDataRef,
					UserDataRef: &v1alpha2.UserDataRef{Kind: v1alpha2.UserDataRefKindSecret, Name: name},
				},
			},
		}
	}

	newReader := func(vm *v1alpha2.VirtualMachine, secretNames ...string) *unifiedManifestReader {
		manifests := []RawManifest{rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineKind, vm)}
		for _, n := range secretNames {
			manifests = append(manifests, rawManifest("v1", "Secret", &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: n}}))
		}
		return &unifiedManifestReader{client: &fakeManifestDownloader{manifests: manifests}, contentName: "content-1"}
	}

	// The capture side declares at most one Secret target today, so taking the first one happened to
	// work; resolving the name from the VM keeps it working once it declares more.
	It("picks the Secret the VM's provisioning references, not the first one", func() {
		reader := newReader(vmWithUserDataRef("cloud-init"), "some-other-secret", "cloud-init")

		secret, err := reader.RestoreProvisioner(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(secret.Name).To(Equal("cloud-init"))
	})

	It("returns nil when the VM has no provisioning Secret to restore", func() {
		reader := newReader(&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm"}}, "unrelated-secret")

		secret, err := reader.RestoreProvisioner(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(secret).To(BeNil())
	})

	It("errors when the referenced provisioner Secret was not captured", func() {
		reader := newReader(vmWithUserDataRef("cloud-init"), "some-other-secret")

		_, err := reader.RestoreProvisioner(context.Background())
		Expect(err).To(MatchError(ContainSubstring(`provisioner Secret "cloud-init"`)))
	})
})

var _ = Describe("NewManifestReader", func() {
	newVMSnapshot := func(mutate func(*v1alpha2.VirtualMachineSnapshot)) *v1alpha2.VirtualMachineSnapshot {
		vms := &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				VirtualMachineSnapshotSecretName: "snapshot-secret",
			},
		}
		mutate(vms)
		return vms
	}

	It("reports a snapshot with status.captureState as unified-captured", func() {
		Expect(IsUnifiedCapture(newVMSnapshot(func(vms *v1alpha2.VirtualMachineSnapshot) {
			vms.Status.CaptureState = &v1alpha2.UnifiedSnapshotterCaptureState{}
		}))).To(BeTrue())

		Expect(IsUnifiedCapture(newVMSnapshot(func(*v1alpha2.VirtualMachineSnapshot) {}))).To(BeFalse())
		Expect(IsUnifiedCapture(nil)).To(BeFalse())
	})

	// The annotation records the opt-in request, not the capture that happened: routing on it would send
	// a legacy-captured snapshot down the SnapshotContent path, where there is no content to read.
	It("reads from the snapshot Secret when only the opt-in annotation is set", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vms := newVMSnapshot(func(vms *v1alpha2.VirtualMachineSnapshot) {
			vms.Annotations = map[string]string{v1alpha2.AnnUseUnifiedSnapshotter: ""}
		})

		reader, err := NewManifestReader(context.Background(), fakeClient, vms)
		Expect(reader).To(BeNil())
		// Took the Secret branch and found no Secret — rather than reaching for a SnapshotContent.
		Expect(err).To(MatchError(ContainSubstring(`restorer secret "snapshot-secret" is not found`)))
	})

	It("requeues a unified-captured snapshot that has no bound SnapshotContent yet", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vms := newVMSnapshot(func(vms *v1alpha2.VirtualMachineSnapshot) {
			vms.Status.CaptureState = &v1alpha2.UnifiedSnapshotterCaptureState{}
		})

		reader, err := NewManifestReader(context.Background(), fakeClient, vms)
		Expect(reader).To(BeNil())
		Expect(err).To(MatchError(common.ErrQueueing))
	})
})

// The state-snapshotter core enforces this handshake on its own namespaced route and documents why:
// status.boundSnapshotContentName is writable on the snapshot side, so without the reverse reference a
// caller can aim it at a foreign cluster-scoped content and read its manifests — including the captured
// VM's provisioner Secret. The snapshotcontents/<name>/manifests-download route we use carries no such
// check of its own.
// A VirtualDiskSnapshot child's own bound SnapshotContent carries the source VirtualDisk manifest
// verbatim — ownerReferences, annotations and labels included. That is what the built-in mechanism keeps
// on the CSI VolumeSnapshot it creates, and what a unified capture would otherwise have no home for.
var _ = Describe("CapturedVirtualDisk", func() {
	const (
		contentName    = "vds-content-1"
		vdSnapshotUID  = "33333333-3333-3333-3333-333333333333"
		vdSnapshotName = "vdsnapshot"
	)

	newVDSnapshot := func(boundContentName string) *v1alpha2.VirtualDiskSnapshot {
		return &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: vdSnapshotName, Namespace: "default", UID: types.UID(vdSnapshotUID)},
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				CaptureState:             &v1alpha2.UnifiedSnapshotterCaptureState{},
				BoundSnapshotContentName: boundContentName,
			},
		}
	}

	newClient := func(ref *ssstoragev1alpha1.SnapshotSubjectRef) client.Client {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(ssstoragev1alpha1.AddToScheme(scheme)).To(Succeed())

		builder := fake.NewClientBuilder().WithScheme(scheme)
		if ref != nil {
			builder = builder.WithObjects(&ssstoragev1alpha1.SnapshotContent{
				ObjectMeta: metav1.ObjectMeta{Name: contentName},
				Spec:       ssstoragev1alpha1.SnapshotContentSpec{SnapshotRef: ref},
			})
		}
		return builder.Build()
	}

	It("leaves a built-in capture to the VolumeSnapshot route", func() {
		vdSnapshot := newVDSnapshot(contentName)
		vdSnapshot.Status.CaptureState = nil

		vd, err := CapturedVirtualDisk(context.Background(), newClient(nil), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(vd).To(BeNil())
	})

	It("requeues while the child has no bound content yet", func() {
		_, err := CapturedVirtualDisk(context.Background(), newClient(nil), newVDSnapshot(""))
		Expect(err).To(MatchError(common.ErrQueueing))
	})

	It("gives up on a bound content that does not exist", func() {
		_, err := CapturedVirtualDisk(context.Background(), newClient(nil), newVDSnapshot(contentName))
		Expect(err).NotTo(MatchError(common.ErrQueueing))
		Expect(err).To(MatchError(ContainSubstring("does not exist")))
	})

	// Same anti-spoofing rule as for the VirtualMachineSnapshot's content: status.boundSnapshotContentName
	// is writable on the snapshot side, and the core does not verify the back-reference on this route.
	It("rejects a content bound to a different disk snapshot", func() {
		_, err := CapturedVirtualDisk(context.Background(), newClient(&ssstoragev1alpha1.SnapshotSubjectRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskSnapshotKind,
			Namespace:  "default",
			Name:       "someone-elses-vdsnapshot",
		}), newVDSnapshot(contentName))
		Expect(err).To(MatchError(ContainSubstring("does not point back at")))
	})

	It("rejects a content that points at the parent VirtualMachineSnapshot instead", func() {
		_, err := CapturedVirtualDisk(context.Background(), newClient(&ssstoragev1alpha1.SnapshotSubjectRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineSnapshotKind,
			Namespace:  "default",
			Name:       vdSnapshotName,
		}), newVDSnapshot(contentName))
		Expect(err).To(MatchError(ContainSubstring("does not point back at")))
	})

	It("rejects a content re-pointed at a re-created disk snapshot of the same name", func() {
		_, err := CapturedVirtualDisk(context.Background(), newClient(&ssstoragev1alpha1.SnapshotSubjectRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskSnapshotKind,
			Namespace:  "default",
			Name:       vdSnapshotName,
			UID:        "44444444-4444-4444-4444-444444444444",
		}), newVDSnapshot(contentName))
		Expect(err).To(MatchError(ContainSubstring("does not match")))
	})
})

var _ = Describe("capturedVirtualDiskFrom", func() {
	It("returns the captured VirtualDisk with its metadata intact", func() {
		captured := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "vd",
				Namespace:   "default",
				Annotations: map[string]string{"custom-key": "custom-value"},
				Labels:      map[string]string{"env": "prod"},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.VirtualMachineKind,
					Name:       "vm",
				}},
			},
		}
		downloader := &fakeManifestDownloader{manifests: []RawManifest{
			rawManifest(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualDiskKind, captured),
		}}

		vd, err := capturedVirtualDiskFrom(context.Background(), downloader, "vds-content-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(vd.Annotations).To(HaveKeyWithValue("custom-key", "custom-value"))
		Expect(vd.Labels).To(HaveKeyWithValue("env", "prod"))
		Expect(hasVirtualMachineOwner(vd)).To(BeTrue())
	})

	It("ignores a same-Kind manifest from a foreign group", func() {
		downloader := &fakeManifestDownloader{manifests: []RawManifest{
			rawManifest("internal.virtualization.deckhouse.io/v1", v1alpha2.VirtualDiskKind,
				&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "not-ours"}}),
		}}

		_, err := capturedVirtualDiskFrom(context.Background(), downloader, "vds-content-1")
		Expect(err).To(MatchError(ContainSubstring("no VirtualDisk manifest found")))
	})

	It("propagates a download failure", func() {
		downloader := &fakeManifestDownloader{err: errors.New("boom")}

		_, err := capturedVirtualDiskFrom(context.Background(), downloader, "vds-content-1")
		Expect(err).To(MatchError(ContainSubstring("boom")))
	})
})

var _ = Describe("NewManifestReader SnapshotContent back-reference", func() {
	const (
		contentName = "content-1"
		snapshotUID = "11111111-1111-1111-1111-111111111111"
	)

	newVMSnapshot := func() *v1alpha2.VirtualMachineSnapshot {
		return &v1alpha2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default", UID: types.UID(snapshotUID)},
			Status: v1alpha2.VirtualMachineSnapshotStatus{
				CaptureState:             &v1alpha2.UnifiedSnapshotterCaptureState{},
				BoundSnapshotContentName: contentName,
			},
		}
	}

	matchingRef := func() *ssstoragev1alpha1.SnapshotSubjectRef {
		return &ssstoragev1alpha1.SnapshotSubjectRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineSnapshotKind,
			Namespace:  "default",
			Name:       "snapshot",
			UID:        types.UID(snapshotUID),
		}
	}

	newClient := func(ref *ssstoragev1alpha1.SnapshotSubjectRef) client.Client {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(ssstoragev1alpha1.AddToScheme(scheme)).To(Succeed())

		builder := fake.NewClientBuilder().WithScheme(scheme)
		if ref != nil {
			builder = builder.WithObjects(&ssstoragev1alpha1.SnapshotContent{
				ObjectMeta: metav1.ObjectMeta{Name: contentName},
				Spec:       ssstoragev1alpha1.SnapshotContentSpec{SnapshotRef: ref},
			})
		}
		return builder.Build()
	}

	BeforeEach(func() {
		Expect(InitContentClient(&rest.Config{Host: "https://kubernetes.default.svc"})).To(Succeed())
	})

	It("accepts a content that points back at this snapshot", func() {
		reader, err := NewManifestReader(context.Background(), newClient(matchingRef()), newVMSnapshot())
		Expect(err).NotTo(HaveOccurred())
		Expect(reader).NotTo(BeNil())
	})

	It("rejects a content bound to a different snapshot", func() {
		ref := matchingRef()
		ref.Name = "someone-elses-snapshot"

		_, err := NewManifestReader(context.Background(), newClient(ref), newVMSnapshot())
		Expect(err).To(MatchError(ContainSubstring("does not point back at")))
	})

	It("rejects a content bound to a snapshot in another namespace", func() {
		ref := matchingRef()
		ref.Namespace = "other-tenant"

		_, err := NewManifestReader(context.Background(), newClient(ref), newVMSnapshot())
		Expect(err).To(MatchError(ContainSubstring("does not point back at")))
	})

	It("rejects a content re-pointed at a re-created snapshot of the same name", func() {
		ref := matchingRef()
		ref.UID = "22222222-2222-2222-2222-222222222222"

		_, err := NewManifestReader(context.Background(), newClient(ref), newVMSnapshot())
		Expect(err).To(MatchError(ContainSubstring("does not match")))
	})

	It("rejects a content with no back-reference at all", func() {
		_, err := NewManifestReader(context.Background(), newClient(&ssstoragev1alpha1.SnapshotSubjectRef{}), newVMSnapshot())
		Expect(err).To(MatchError(ContainSubstring("does not point back at")))
	})

	// A named-but-absent content is the end of the road, not a wait: parking the VirtualMachineOperation
	// in InProgress would block every later operation on the VirtualMachine behind the vmop webhook.
	It("gives up on a bound content that does not exist", func() {
		_, err := NewManifestReader(context.Background(), newClient(nil), newVMSnapshot())
		Expect(err).NotTo(MatchError(common.ErrQueueing))
		Expect(err).To(MatchError(ContainSubstring(`SnapshotContent "content-1" named by VirtualMachineSnapshot default/snapshot`)))
	})
})
