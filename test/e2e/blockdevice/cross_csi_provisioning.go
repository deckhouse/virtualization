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

package blockdevice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// Creating a block device from a source that lives on a storage class backed by a
// different CSI driver is not supported. The virtualization-controller admission
// webhooks must reject such requests at creation time.
var _ = Describe("CrossCSIDriverProvisioning", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
	var (
		f   *framework.Framework
		ctx context.Context

		scPtr          *string
		differentSCPtr *string
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("cross-csi")
		f.Before()
		DeferCleanup(f.After)

		// Sources live on the main StorageClass, whatever its volume binding mode:
		// images provision standalone on any mode, and WaitForFirstConsumer disks
		// are provisioned through a temporary VM consumer (see readySourceVD).
		scPtr = mainStorageClass()
		differentSCPtr = differentCSIDriverStorageClass()
	})

	It("rejects creating a VirtualDisk from a VirtualImage backed by a different CSI driver", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
		sourceVI := vibuilder.New(
			vibuilder.WithName("vi-source-main-csi"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)
		sourceVI.Spec.PersistentVolumeClaim.StorageClass = scPtr
		createVirtualImageAndWait(ctx, f, sourceVI)

		target := vdbuilder.New(
			vdbuilder.WithName("vd-from-vi-cross-csi"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualImage, sourceVI.Name),
			vdbuilder.WithStorageClass(differentSCPtr),
		)

		expectCrossCSIRejection(ctx, f, target)
	})

	It("rejects creating a VirtualImage from a VirtualDisk backed by a different CSI driver", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
		sourceVD := readySourceVD(ctx, f, scPtr, "vd-source-main-csi")

		target := vibuilder.New(
			vibuilder.WithName("vi-from-vd-cross-csi"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, sourceVD.Name),
		)
		target.Spec.PersistentVolumeClaim.StorageClass = differentSCPtr

		expectCrossCSIRejection(ctx, f, target)
	})

	It("rejects creating a VirtualImage from a VirtualImage backed by a different CSI driver", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
		sourceVI := vibuilder.New(
			vibuilder.WithName("vi-source-main-csi"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)
		sourceVI.Spec.PersistentVolumeClaim.StorageClass = scPtr
		createVirtualImageAndWait(ctx, f, sourceVI)

		target := vibuilder.New(
			vibuilder.WithName("vi-from-vi-cross-csi"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, sourceVI.Name),
		)
		target.Spec.PersistentVolumeClaim.StorageClass = differentSCPtr

		expectCrossCSIRejection(ctx, f, target)
	})

	// The webhook legitimately admits a resource when the source provisioner is not
	// determinable yet (e.g. the referenced image does not exist at creation time).
	// This spec pins the reconcile-time guard: once the source becomes ready on a
	// different CSI driver, provisioning must fail with a clear message instead of
	// starting a PVC-to-PVC copy that can never succeed.
	It("fails provisioning a VirtualImage when the cross-CSI image source appears after creation", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
		const sourceName = "vi-late-source-main-csi"

		target := vibuilder.New(
			vibuilder.WithName("vi-from-late-vi-cross-csi"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, sourceName),
		)
		target.Spec.PersistentVolumeClaim.StorageClass = differentSCPtr

		By("Creating the target VirtualImage while the source image does not exist yet", func() {
			err := f.CreateWithDeferredDeletion(ctx, target)
			Expect(err).NotTo(HaveOccurred(),
				"creation must be admitted while the source provisioner is not determinable")
		})

		sourceVI := vibuilder.New(
			vibuilder.WithName(sourceName),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)
		sourceVI.Spec.PersistentVolumeClaim.StorageClass = scPtr
		createVirtualImageAndWait(ctx, f, sourceVI)

		By("Expecting the provisioning to fail on the reconcile-time cross-provider check", func() {
			util.UntilObjectPhase(ctx, string(v1alpha2.ImageFailed), framework.LongTimeout, target)

			err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(target), target)
			Expect(err).NotTo(HaveOccurred())
			Expect(readyConditionMessage(target.Status.Conditions)).To(ContainSubstring("Cross-provider PVC copy is not supported"))
		})
	})

	Context("with snapshots", Label(precheck.PrecheckSnapshot), func() {
		It("rejects creating a VirtualDisk from a VirtualDiskSnapshot backed by a different CSI driver", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
			snapshot := createSourceSnapshotOnMainSC(ctx, f, scPtr, "vd-source-for-snap-cross-csi", "vdsnapshot-source-cross-csi-vd")

			target := vdbuilder.New(
				vdbuilder.WithName("vd-from-snapshot-cross-csi"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot, snapshot.Name),
				vdbuilder.WithStorageClass(differentSCPtr),
			)

			expectCrossCSIRejection(ctx, f, target)
		})

		It("rejects creating a VirtualImage from a VirtualDiskSnapshot backed by a different CSI driver", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
			snapshot := createSourceSnapshotOnMainSC(ctx, f, scPtr, "vd-source-for-snap-cross-csi", "vdsnapshot-source-cross-csi-vi")

			target := vibuilder.New(
				vibuilder.WithName("vi-from-snapshot-cross-csi"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, snapshot.Name),
			)
			target.Spec.PersistentVolumeClaim.StorageClass = differentSCPtr

			expectCrossCSIRejection(ctx, f, target)
		})

		// The webhooks legitimately admit a resource when the source provisioner is not
		// determinable yet (e.g. the referenced snapshot does not exist at creation
		// time). These specs pin the reconcile-time guard: once the snapshot becomes
		// ready on a different CSI driver, provisioning must fail with a clear message
		// instead of creating a PVC that can never be populated.
		It("fails provisioning a VirtualDisk when the cross-CSI snapshot source appears after creation", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
			const snapshotName = "vdsnapshot-late-cross-csi-vd"

			target := vdbuilder.New(
				vdbuilder.WithName("vd-from-late-snapshot-cross-csi"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot, snapshotName),
				vdbuilder.WithStorageClass(differentSCPtr),
			)

			By("Creating the target VirtualDisk while the source snapshot does not exist yet", func() {
				err := f.CreateWithDeferredDeletion(ctx, target)
				Expect(err).NotTo(HaveOccurred(),
					"creation must be admitted while the source provisioner is not determinable")
			})

			createSourceSnapshotOnMainSC(ctx, f, scPtr, "vd-source-for-late-snap-cross-csi", snapshotName)

			By("Expecting the provisioning to fail on the reconcile-time cross-provider check", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.DiskFailed), framework.LongTimeout, target)

				err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(target), target)
				Expect(err).NotTo(HaveOccurred())
				Expect(readyConditionMessage(target.Status.Conditions)).To(ContainSubstring("Cross-provider snapshot restore is not supported"))
			})
		})

		It("fails provisioning a VirtualImage when the cross-CSI snapshot source appears after creation", Label(precheck.PrecheckDifferentCSIDriverStorageClass), func() {
			const snapshotName = "vdsnapshot-late-cross-csi-vi"

			target := vibuilder.New(
				vibuilder.WithName("vi-from-late-snapshot-cross-csi"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, snapshotName),
			)
			target.Spec.PersistentVolumeClaim.StorageClass = differentSCPtr

			By("Creating the target VirtualImage while the source snapshot does not exist yet", func() {
				err := f.CreateWithDeferredDeletion(ctx, target)
				Expect(err).NotTo(HaveOccurred(),
					"creation must be admitted while the source provisioner is not determinable")
			})

			createSourceSnapshotOnMainSC(ctx, f, scPtr, "vd-source-for-late-snap-cross-csi", snapshotName)

			By("Expecting the provisioning to fail on the reconcile-time cross-provider check", func() {
				util.UntilObjectPhase(ctx, string(v1alpha2.ImageFailed), framework.LongTimeout, target)

				err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(target), target)
				Expect(err).NotTo(HaveOccurred())
				Expect(readyConditionMessage(target.Status.Conditions)).To(ContainSubstring("Cross-provider snapshot restore is not supported"))
			})
		})
	})
})

// readyConditionMessage returns the message of the Ready condition, or an empty
// string when the condition is not present.
func readyConditionMessage(conds []metav1.Condition) string {
	for _, cond := range conds {
		if cond.Type == "Ready" {
			return cond.Message
		}
	}
	return ""
}

// mainStorageClass returns a pointer to the name of the main StorageClass the suite is
// pointed at (STORAGE_CLASS_NAME or the cluster default), with no constraint on its
// volume binding mode. Its presence is enforced by
// precheck.PrecheckDifferentCSIDriverStorageClass.
func mainStorageClass() *string {
	GinkgoHelper()

	sc := framework.GetConfig().StorageClass.TemplateStorageClass
	Expect(sc).NotTo(BeNil(),
		"main StorageClass not found: set %s or configure a default StorageClass (enforced by the %q precheck)",
		config.StorageClassNameEnv, precheck.PrecheckDifferentCSIDriverStorageClass)

	return ptr.To(sc.Name)
}

// readySourceVD provisions a source VirtualDisk on sc and returns it Ready and
// unattached, regardless of the storage class volume binding mode: an Immediate disk
// provisions standalone, while a WaitForFirstConsumer one is provisioned through a
// temporary VirtualMachine consumer that is deleted once the disk detaches (see
// createSourceVirtualDiskAndWait).
func readySourceVD(ctx context.Context, f *framework.Framework, sc *string, name string) *v1alpha2.VirtualDisk {
	GinkgoHelper()

	if storageClassIsWaitForFirstConsumer(ctx, f, ptr.Deref(sc, "")) {
		return createSourceVirtualDiskAndWait(ctx, f, name, sc)
	}

	sourceVD := vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(f.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{URL: object.ImageTestDataQCOW}),
		vdbuilder.WithStorageClass(sc),
	)
	createVirtualDiskAndWait(ctx, f, sourceVD)

	return sourceVD
}

// differentCSIDriverStorageClass returns a pointer to the name of a StorageClass backed
// by a different CSI driver than the main one. Its presence and distinct CSI driver are
// enforced by precheck.PrecheckDifferentCSIDriverStorageClass.
func differentCSIDriverStorageClass() *string {
	GinkgoHelper()

	sc := framework.GetConfig().StorageClass.DifferentCSIDriverStorageClass
	Expect(sc).NotTo(BeNil(),
		"no StorageClass with a CSI driver different from the main one was found "+
			"(discovered automatically; enforced by the %q precheck)",
		precheck.PrecheckDifferentCSIDriverStorageClass)

	return ptr.To(sc.Name)
}

// createSourceSnapshotOnMainSC provisions a VirtualDisk on the main storage class and a
// ready VirtualDiskSnapshot of it, returning the snapshot. The disk is unattached by the
// time it is snapshotted, so the snapshot is consistent on any volume binding mode.
func createSourceSnapshotOnMainSC(ctx context.Context, f *framework.Framework, sc *string, vdName, snapshotName string) *v1alpha2.VirtualDiskSnapshot {
	GinkgoHelper()

	sourceVD := readySourceVD(ctx, f, sc, vdName)

	snapshot := vdsnapshotbuilder.New(
		vdsnapshotbuilder.WithName(snapshotName),
		vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
		vdsnapshotbuilder.WithVirtualDiskName(sourceVD.Name),
		vdsnapshotbuilder.WithRequiredConsistency(true),
	)

	By("Creating the source VirtualDiskSnapshot", func() {
		err := f.CreateWithDeferredDeletion(ctx, snapshot)
		Expect(err).NotTo(HaveOccurred())

		util.UntilVDSnapshotsReady(ctx, f, framework.LongTimeout, snapshot)
	})

	return snapshot
}

// expectCrossCSIRejection attempts to create obj and asserts that the admission webhook
// rejects it because the source lives on a different CSI driver.
func expectCrossCSIRejection(ctx context.Context, f *framework.Framework, obj crclient.Object) {
	GinkgoHelper()

	By("Expecting the webhook to reject creation from a source on a different CSI driver", func() {
		err := f.CreateWithDeferredDeletion(ctx, obj)
		Expect(err).To(HaveOccurred(), "creation from a source on a different CSI driver must be rejected by the webhook")
		Expect(err.Error()).To(ContainSubstring("provisioner"),
			"the rejection error should explain the storage class provisioner mismatch")
	})
}
