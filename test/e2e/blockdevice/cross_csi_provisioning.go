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
		sourceVD := vdbuilder.New(
			vdbuilder.WithName("vd-source-main-csi"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{URL: object.ImageTestDataQCOW}),
			vdbuilder.WithStorageClass(scPtr),
		)
		createVirtualDiskAndWait(ctx, f, sourceVD)

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
	})
})

// differentCSIDriverStorageClass returns a pointer to the name of a StorageClass backed
// by a different CSI driver than the main one. Its presence and distinct CSI driver are
// enforced by precheck.PrecheckDifferentCSIDriverStorageClass.
func differentCSIDriverStorageClass() *string {
	GinkgoHelper()

	sc := framework.GetConfig().StorageClass.DifferentCSIDriverStorageClass
	Expect(sc).NotTo(BeNil(),
		"different-CSI-driver StorageClass not found: annotate a StorageClass with %s=true (enforced by the %q precheck)",
		config.DifferentCSIDriverStorageClassAnnotation, precheck.PrecheckDifferentCSIDriverStorageClass)

	return ptr.To(sc.Name)
}

// createSourceSnapshotOnMainSC provisions a VirtualDisk on the main storage class and a
// ready VirtualDiskSnapshot of it, returning the snapshot.
func createSourceSnapshotOnMainSC(ctx context.Context, f *framework.Framework, sc *string, vdName, snapshotName string) *v1alpha2.VirtualDiskSnapshot {
	GinkgoHelper()

	sourceVD := vdbuilder.New(
		vdbuilder.WithName(vdName),
		vdbuilder.WithNamespace(f.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{URL: object.ImageTestDataQCOW}),
		vdbuilder.WithStorageClass(sc),
	)
	createVirtualDiskAndWait(ctx, f, sourceVD)

	snapshot := vdsnapshotbuilder.New(
		vdsnapshotbuilder.WithName(snapshotName),
		vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
		vdsnapshotbuilder.WithVirtualDiskName(sourceVD.Name),
		vdsnapshotbuilder.WithRequiredConsistency(true),
	)

	By("Creating the source VirtualDiskSnapshot", func() {
		err := f.CreateWithDeferredDeletion(ctx, snapshot)
		Expect(err).NotTo(HaveOccurred())

		util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.LongTimeout, snapshot)
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
