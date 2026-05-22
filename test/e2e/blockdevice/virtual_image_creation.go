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
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualImageCreation", Ordered, Label(precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context

		scPtr *string
	)

	BeforeAll(func() {
		ctx = context.Background()
		f = framework.NewFramework("vi-creation")
		f.Before()
		DeferCleanup(f.After)

		scPtr = ptr.To(vdCreationStorageClass)
	})

	It("provisions VirtualImages on DVCR and PVC from HTTP data source", Label(precheck.NoPrecheck), func() {
		viDVCR := newVirtualImageOnDVCR("vi-http",
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)
		viPVC := newVirtualImageOnPVC("vi-pvc-http", scPtr,
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)

		createVirtualImageAndWait(ctx, f, viDVCR)

		createVirtualImageAndWait(ctx, f, viPVC)
	})

	It("provisions VirtualImages on DVCR and PVC from Upload data source", Label(precheck.NoPrecheck), func() {
		viDVCR := newVirtualImageOnDVCR("vi-upload",
			vibuilder.WithDatasource(v1alpha2.VirtualImageDataSource{
				Type: v1alpha2.DataSourceTypeUpload,
			}),
		)
		viPVC := newVirtualImageOnPVC("vi-pvc-upload", scPtr,
			vibuilder.WithDatasource(v1alpha2.VirtualImageDataSource{
				Type: v1alpha2.DataSourceTypeUpload,
			}),
		)

		var uploadFilePath string
		By("Downloading source image to upload", func() {
			var err error
			uploadFilePath, err = downloadImageToTempFile(object.ImageTestDataQCOW)
			Expect(err).NotTo(HaveOccurred(), "failed to download upload source image")
			DeferCleanup(func() {
				removeErr := os.Remove(uploadFilePath)
				Expect(removeErr == nil || errors.Is(removeErr, os.ErrNotExist)).To(BeTrue(),
					"failed to remove upload source file %q: %v", uploadFilePath, removeErr)
			})
		})

		uploadVirtualImageAndWait(ctx, f, viDVCR, uploadFilePath)

		uploadVirtualImageAndWait(ctx, f, viPVC, uploadFilePath)
	})

	It("provisions VirtualImages on DVCR and PVC from ContainerImage (registry) data source", Label(precheck.NoPrecheck), func() {
		viDVCR := newVirtualImageOnDVCR("vi-registry",
			vibuilder.WithDataSourceContainerImage(object.ImageURLContainerImage, v1alpha2.ImagePullSecretName{}, nil),
		)
		viPVC := newVirtualImageOnPVC("vi-pvc-registry", scPtr,
			vibuilder.WithDataSourceContainerImage(object.ImageURLContainerImage, v1alpha2.ImagePullSecretName{}, nil),
		)

		createVirtualImageAndWait(ctx, f, viDVCR)

		createVirtualImageAndWait(ctx, f, viPVC)
	})

	It("provisions VirtualImages on DVCR and PVC from a VirtualDisk", Label(precheck.NoPrecheck), func() {
		vd := createHTTPVirtualDiskAndWait(ctx, f, "vd-source-for-vi", scPtr)

		viDVCR := newVirtualImageOnDVCR("vi-from-vd",
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
		)
		viPVC := newVirtualImageOnPVC("vi-pvc-from-vd", scPtr,
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
		)

		createVirtualImageAndWait(ctx, f, viDVCR)

		createVirtualImageAndWait(ctx, f, viPVC)
	})

	It("provisions VirtualImages on DVCR and PVC from a ClusterVirtualImage", Label(precheck.NoPrecheck), func() {
		viDVCR := newVirtualImageOnDVCR("vi-from-cvi",
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVITestDataQCOW),
		)
		viPVC := newVirtualImageOnPVC("vi-pvc-from-cvi", scPtr,
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVITestDataQCOW),
		)

		createVirtualImageAndWait(ctx, f, viDVCR)

		createVirtualImageAndWait(ctx, f, viPVC)
	})

	It("provisions VirtualImages on DVCR and PVC from a VirtualImage on DVCR", Label(precheck.NoPrecheck), func() {
		baseVI := newVirtualImageOnDVCR("vi-source-dvcr",
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)
		createVirtualImageAndWait(ctx, f, baseVI)

		viDVCR := newVirtualImageOnDVCR("vi-from-vi-dvcr",
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
		)
		viPVC := newVirtualImageOnPVC("vi-pvc-from-vi-dvcr", scPtr,
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
		)

		createVirtualImageAndWait(ctx, f, viDVCR)

		createVirtualImageAndWait(ctx, f, viPVC)
	})

	It("provisions VirtualImages on DVCR and PVC from a VirtualImage on PVC", Label(precheck.NoPrecheck), func() {
		baseVI := newVirtualImageOnPVC("vi-source-pvc", scPtr,
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)
		createVirtualImageAndWait(ctx, f, baseVI)

		viDVCR := newVirtualImageOnDVCR("vi-from-vi-pvc",
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
		)
		viPVC := newVirtualImageOnPVC("vi-pvc-from-vi-pvc", scPtr,
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
		)

		createVirtualImageAndWait(ctx, f, viDVCR)

		createVirtualImageAndWait(ctx, f, viPVC)
	})

	Context("with snapshots", Label(precheck.PrecheckSnapshot), func() {
		It("provisions VirtualImages on DVCR and PVC from a VirtualDiskSnapshot", func() {
			vd := createHTTPVirtualDiskAndWait(ctx, f, "vd-source-for-vi-snapshot", scPtr)

			vdSnapshot := vdsnapshotbuilder.New(
				vdsnapshotbuilder.WithName("vdsnapshot-source-for-vi"),
				vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vdsnapshotbuilder.WithVirtualDiskName(vd.Name),
				vdsnapshotbuilder.WithRequiredConsistency(true),
			)

			By("Creating VirtualDiskSnapshot", func() {
				err := f.CreateWithDeferredDeletion(ctx, vdSnapshot)
				Expect(err).NotTo(HaveOccurred())

				util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.LongTimeout, vdSnapshot)
			})

			viDVCR := newVirtualImageOnDVCR("vi-from-vdsnapshot",
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			viPVC := newVirtualImageOnPVC("vi-pvc-from-vdsnapshot", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)

			createVirtualImageAndWait(ctx, f, viDVCR)

			createVirtualImageAndWait(ctx, f, viPVC)
		})
	})
})

func newVirtualImageOnDVCR(name string, opts ...vibuilder.Option) *v1alpha2.VirtualImage {
	baseOpts := []vibuilder.Option{
		vibuilder.WithName(name),
		vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
	}
	baseOpts = append(baseOpts, opts...)
	return vibuilder.New(baseOpts...)
}

func newVirtualImageOnPVC(name string, sc *string, opts ...vibuilder.Option) *v1alpha2.VirtualImage {
	vi := newVirtualImageOnDVCR(name,
		append([]vibuilder.Option{vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim)}, opts...)...,
	)
	vi.Spec.PersistentVolumeClaim.StorageClass = sc
	return vi
}

func createVirtualImageAndWait(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage) {
	GinkgoHelper()

	vi.Namespace = f.Namespace().Name
	obs := viobs.StartObserver(ctx, f, vi)
	obs.Never(viobs.BeFailed())
	obs.Always(viobs.HaveNonDecreasingProgress())

	By("Creating VirtualImage on "+virtualImageStorageName(vi), func() {
		err := f.CreateWithDeferredDeletion(ctx, vi)
		Expect(err).NotTo(HaveOccurred())
	})

	err := obs.WaitFor(viobs.BeReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
}

func uploadVirtualImageAndWait(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage, uploadFilePath string) {
	GinkgoHelper()

	vi.Namespace = f.Namespace().Name
	obs := viobs.StartObserver(ctx, f, vi)
	obs.Never(viobs.BeFailed())
	obs.Always(viobs.HaveNonDecreasingProgress())

	By("Creating VirtualImage on "+virtualImageStorageName(vi), func() {
		err := f.CreateWithDeferredDeletion(ctx, vi)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Waiting for the VirtualImage to expose upload URLs", func() {
		err := obs.WaitFor(viobs.BeReadyForUserUpload(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Allowing ingress-nginx to reach the uploader pod (workaround)", func() {
		err := allowIngressNginxToUploaderNetworkPolicy(ctx, f, vi.Namespace, vi.UID)
		Expect(err).NotTo(HaveOccurred(), "failed to patch uploader NetworkPolicy")
	})

	By("Uploading data to the VirtualImage", func() {
		err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vi), vi)
		Expect(err).NotTo(HaveOccurred())
		Expect(vi.Status.ImageUploadURLs).NotTo(BeNil())
		Expect(vi.Status.ImageUploadURLs.External).NotTo(BeEmpty())

		err = doRetriableUploadAttempt(vi.Status.ImageUploadURLs.External, uploadFilePath)
		Expect(err).NotTo(HaveOccurred(), "upload should succeed")
	})

	err := obs.WaitFor(viobs.BeReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
}

func createHTTPVirtualDiskAndWait(ctx context.Context, f *framework.Framework, name string, sc *string) *v1alpha2.VirtualDisk {
	GinkgoHelper()

	vd := vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(f.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{URL: object.ImageTestDataQCOW}),
		vdbuilder.WithStorageClass(sc),
	)

	createVirtualDiskAndWait(ctx, f, vd)

	return vd
}

func virtualImageStorageName(vi *v1alpha2.VirtualImage) string {
	switch vi.Spec.Storage {
	case v1alpha2.StorageContainerRegistry:
		return "DVCR"
	case v1alpha2.StoragePersistentVolumeClaim:
		return "PVC"
	default:
		return string(vi.Spec.Storage)
	}
}
