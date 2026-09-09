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
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	cviobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/cvi"
	vdsnapshotobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vdsnapshot"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("ClusterVirtualImageCreation", Label(
	label.SIGStorage,
	precheck.PrecheckDefaultStorageClass,
	precheck.PrecheckSnapshot,
), func() {
	var (
		f *framework.Framework

		scPtr *string
	)

	// A ClusterVirtualImage always stores its data on DVCR, but each spec still
	// needs a dedicated Project: it hosts the namespaced sources (VirtualImage,
	// VirtualDisk, VirtualDiskSnapshot) and the VirtualMachine that verifies the
	// image boots.
	setup := func(ctx context.Context) {
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "cvi-creation")

		scPtr = defaultStorageClass()
	}

	Context("from HTTP data source", func() {
		BeforeEach(setup)

		It("provisions a ClusterVirtualImage", func(ctx context.Context) {
			cvi := newClusterVirtualImage(f, "http",
				cvibuilder.WithDataSourceHTTP(object.ImageURLCustomBIOS, nil, nil),
			)
			createClusterVirtualImageAndRunVM(ctx, f, cvi)
		})
	})

	Context("from ContainerImage (registry) data source", func() {
		BeforeEach(setup)

		It("provisions a ClusterVirtualImage", func(ctx context.Context) {
			cvi := newClusterVirtualImage(f, "registry",
				cvibuilder.WithDataSourceContainerImage(object.ImageURLCustomContainer, v1alpha2.ImagePullSecret{}, nil),
			)
			createClusterVirtualImageAndRunVM(ctx, f, cvi)
		})
	})

	Context("from a ClusterVirtualImage", func() {
		BeforeEach(setup)

		It("provisions a ClusterVirtualImage", func(ctx context.Context) {
			cvi := newClusterVirtualImage(f, "from-cvi",
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS, ""),
			)
			createClusterVirtualImageAndRunVM(ctx, f, cvi)
		})
	})

	Context("from Upload data source", Ordered, func() {
		var uploadFilePath string

		BeforeAll(func(ctx context.Context) {
			// TODO: Re-enable the Upload spec.
			Skip("skipped as flaky: fix the instability, then remove this skip")

			setup(ctx)

			By("Downloading source image to upload", func() {
				var err error
				uploadFilePath, err = downloadImageToTempFile(object.ImageURLCustomBIOS)
				Expect(err).NotTo(HaveOccurred(), "failed to download upload source image")
				DeferCleanup(func() {
					removeErr := os.Remove(uploadFilePath)
					Expect(removeErr == nil || errors.Is(removeErr, os.ErrNotExist)).To(BeTrue(),
						"failed to remove upload source file %q: %v", uploadFilePath, removeErr)
				})
			})
		})

		It("provisions a ClusterVirtualImage", func(ctx context.Context) {
			cvi := newClusterVirtualImage(f, "upload",
				cvibuilder.WithDatasource(v1alpha2.ClusterVirtualImageDataSource{
					Type: v1alpha2.DataSourceTypeUpload,
				}),
			)
			uploadClusterVirtualImageAndWait(ctx, f, cvi, uploadFilePath)
			runVirtualMachineFromClusterImageDisk(ctx, f, cvi)
		})
	})

	Context("from a VirtualDisk", Ordered, func() {
		BeforeAll(setup)

		It("provisions a ClusterVirtualImage", func(ctx context.Context) {
			vd := createSourceVirtualDiskAndWait(ctx, f, "vd-source-for-cvi", scPtr)

			cvi := newClusterVirtualImage(f, "from-vd",
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk, vd.Name, f.Namespace().Name),
			)
			createClusterVirtualImageAndRunVM(ctx, f, cvi)
		})
	})

	Context("from a VirtualImage on DVCR", Ordered, func() {
		var baseVI *v1alpha2.VirtualImage

		BeforeAll(func(ctx context.Context) {
			setup(ctx)
			baseVI = newVirtualImageOnDVCR("vi-source-dvcr-for-cvi",
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
			)
			createVirtualImageAndWait(ctx, f, baseVI)
		})

		It("provisions a ClusterVirtualImage", func(ctx context.Context) {
			cvi := newClusterVirtualImage(f, "from-vi-dvcr",
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage, baseVI.Name, f.Namespace().Name),
			)
			createClusterVirtualImageAndRunVM(ctx, f, cvi)
		})
	})

	Context("from a VirtualImage on PVC", Ordered, func() {
		var baseVI *v1alpha2.VirtualImage

		BeforeAll(func(ctx context.Context) {
			setup(ctx)
			baseVI = newVirtualImageOnPVC("vi-source-pvc-for-cvi", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
			)
			createVirtualImageAndWait(ctx, f, baseVI)
		})

		It("provisions a ClusterVirtualImage", func(ctx context.Context) {
			cvi := newClusterVirtualImage(f, "from-vi-pvc",
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage, baseVI.Name, f.Namespace().Name),
			)
			// TODO: remove the explicit size once the sizing bug is fixed. The image
			// imported from a block PVC is as large as the actual (extent-rounded)
			// volume, which exceeds the source's reported unpacked size, so a disk
			// with the controller-derived size fails to provision ("A larger PVC is
			// required"). See the restoreCreatedVIDiskSize workaround for the same
			// class of issue in the VirtualImage suite.
			createClusterVirtualImageAndRunVM(ctx, f, cvi,
				vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))),
			)
		})
	})

	Context("from a VirtualDiskSnapshot", Ordered, Label(precheck.PrecheckSnapshot), func() {
		var vdSnapshot *v1alpha2.VirtualDiskSnapshot

		BeforeAll(func(ctx context.Context) {
			// TODO: unskip once snapshotting on the e2e clusters is stable. Taking a
			// VolumeSnapshot intermittently fails at the CSI level (LINSTOR/DRBD:
			// "Failed to suspend IO ... on layer DrbdLayer"), so the source
			// VirtualDiskSnapshot goes Failed before the ClusterVirtualImage is even
			// created. The VirtualDiskSnapshots suite is skipped for the same storage
			// instability.
			Skip("flaky: CSI VolumeSnapshot creation fails intermittently on DRBD (suspend IO); see the TODO above")

			setup(ctx)
			vd := createSourceVirtualDiskAndWait(ctx, f, "vd-source-for-cvi-snapshot", scPtr)

			vdSnapshot = vdsnapshotbuilder.New(
				vdsnapshotbuilder.WithName("vdsnapshot-source-for-cvi"),
				vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vdsnapshotbuilder.WithVirtualDiskName(vd.Name),
				vdsnapshotbuilder.WithRequiredConsistency(true),
			)

			snapObs := vdsnapshotobs.StartObserver(ctx, f, vdSnapshot)
			By("Creating VirtualDiskSnapshot", func() {
				err := f.CreateWithDeferredDeletion(ctx, vdSnapshot)
				Expect(err).NotTo(HaveOccurred())

				err = snapObs.WaitFor(vdsnapshotobs.BeReady(), framework.LongTimeout)
				skipIfCSISnapshotFailed(err)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("provisions a ClusterVirtualImage", func(ctx context.Context) {
			cvi := newClusterVirtualImage(f, "from-vdsnapshot",
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name, f.Namespace().Name),
			)
			createClusterVirtualImageAndRunVM(ctx, f, cvi)
		})
	})
})

// newClusterVirtualImage builds a ClusterVirtualImage whose name is prefixed
// with the spec's unique project name: the resource is cluster-scoped, so the
// prefix keeps parallel test processes from colliding on names.
func newClusterVirtualImage(f *framework.Framework, suffix string, opts ...cvibuilder.Option) *v1alpha2.ClusterVirtualImage {
	baseOpts := []cvibuilder.Option{
		cvibuilder.WithName(f.Namespace().Name + "-" + suffix),
	}
	baseOpts = append(baseOpts, opts...)
	return cvibuilder.New(baseOpts...)
}

func startClusterVirtualImageObserver(ctx context.Context, f *framework.Framework, cvi *v1alpha2.ClusterVirtualImage) cviobs.Observer {
	GinkgoHelper()

	obs := cviobs.StartObserver(ctx, f, cvi)
	obs.Never(cviobs.BeFailed())
	obs.Always(cviobs.HaveValidPhaseTransitions())
	obs.Always(cviobs.HaveValidProgress(cviobs.ProgressExpectations{
		RequireZero:    true,
		RequireHundred: true,
	}))
	obs.Always(cviobs.HaveFormat(expectedClusterVirtualImageFormat(ctx, f, cvi)))

	return obs
}

func createClusterVirtualImageAndWait(ctx context.Context, f *framework.Framework, cvi *v1alpha2.ClusterVirtualImage) {
	GinkgoHelper()

	obs := startClusterVirtualImageObserver(ctx, f, cvi)

	By("Creating ClusterVirtualImage", func() {
		err := f.CreateWithDeferredDeletion(ctx, cvi)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Waiting for the ClusterVirtualImage to be Ready", func() {
		err := obs.WaitFor(cviobs.BeReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})
}

func uploadClusterVirtualImageAndWait(ctx context.Context, f *framework.Framework, cvi *v1alpha2.ClusterVirtualImage, uploadFilePath string) {
	GinkgoHelper()

	obs := startClusterVirtualImageObserver(ctx, f, cvi)

	By("Creating ClusterVirtualImage", func() {
		err := f.CreateWithDeferredDeletion(ctx, cvi)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Waiting for the ClusterVirtualImage to expose upload URLs", func() {
		err := obs.WaitFor(cviobs.BeReadyForUserUpload(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	// The uploader supplements of a cluster-scoped image live in the
	// virtualization-controller namespace, not in the test project.
	By("Allowing ingress-nginx and the controller to reach the uploader pod (workaround)", func() {
		err := allowIngressToUploaderNetworkPolicy(ctx, f, controllerNamespaceLabelValue, cvi.UID)
		Expect(err).NotTo(HaveOccurred(), "failed to patch uploader NetworkPolicy")
	})

	By("Uploading data to the ClusterVirtualImage", func() {
		err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(cvi), cvi)
		Expect(err).NotTo(HaveOccurred())
		Expect(cvi.Status.ImageUploadURLs).NotTo(BeNil())
		Expect(cvi.Status.ImageUploadURLs.External).NotTo(BeEmpty())

		err = doRetriableUploadAttempt(cvi.Status.ImageUploadURLs.External, uploadFilePath)
		Expect(err).NotTo(HaveOccurred(), "upload should succeed")
	})

	By("Waiting for the ClusterVirtualImage to be Ready", func() {
		err := obs.WaitFor(cviobs.BeReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})
}

// createClusterVirtualImageAndRunVM provisions a (qcow2) ClusterVirtualImage,
// provisions a VirtualDisk from it, and boots a VirtualMachine from that disk.
// An image cannot occupy a VM's first block-device slot, so booting always goes
// through a VirtualDisk; the VM is run until it is Running and its guest agent
// is ready.
func createClusterVirtualImageAndRunVM(ctx context.Context, f *framework.Framework, cvi *v1alpha2.ClusterVirtualImage, vdOpts ...vdbuilder.Option) {
	GinkgoHelper()

	createClusterVirtualImageAndWait(ctx, f, cvi)
	runVirtualMachineFromClusterImageDisk(ctx, f, cvi, vdOpts...)
}

// runVirtualMachineFromClusterImageDisk provisions a VirtualDisk from the
// (Ready) ClusterVirtualImage and boots a VirtualMachine from that disk,
// waiting until the VM is Running and its guest agent is ready.
func runVirtualMachineFromClusterImageDisk(ctx context.Context, f *framework.Framework, cvi *v1alpha2.ClusterVirtualImage, vdOpts ...vdbuilder.Option) {
	GinkgoHelper()

	baseOpts := []vdbuilder.Option{
		vdbuilder.WithStorageClass(defaultStorageClass()),
	}
	baseOpts = append(baseOpts, vdOpts...)
	vd := object.NewVDFromCVI("vd-from-cvi", f.Namespace().Name, cvi.Name, baseOpts...)
	createVirtualDiskAndRunVM(ctx, f, vd)
}
