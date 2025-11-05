/*
Copyright 2025 Flant JSC

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

package images

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualImageCreation", func() {
	f := framework.NewFramework("vi-creation")

	BeforeEach(func() {
		f.Before()
		DeferCleanup(f.After)
	})

	It("verifies that images are created successfully", func() {
		const cviPrefix = "v12-e2e"
		var (
			vd                                  *v1alpha2.VirtualDisk
			vdSnapshot                          *v1alpha2.VirtualDiskSnapshot
			cviHttp                             *v1alpha2.ClusterVirtualImage
			cviContainerImage                   *v1alpha2.ClusterVirtualImage
			cviFromCVIHttp                      *v1alpha2.ClusterVirtualImage
			cviFromCVIContainerImage            *v1alpha2.ClusterVirtualImage
			cviFromVD                           *v1alpha2.ClusterVirtualImage
			cviFromVDSnapshot                   *v1alpha2.ClusterVirtualImage
			viHttp                              *v1alpha2.VirtualImage
			viContainerImage                    *v1alpha2.VirtualImage
			viFromCVIHttp                       *v1alpha2.VirtualImage
			viFromCVIContainerImage             *v1alpha2.VirtualImage
			viFromVD                            *v1alpha2.VirtualImage
			viFromVDSnapshot                    *v1alpha2.VirtualImage
			viPvcHttp                           *v1alpha2.VirtualImage
			viPvcContainerImage                 *v1alpha2.VirtualImage
			viPvcFromCVIHttp                    *v1alpha2.VirtualImage
			viPvcFromCVIContainerImage          *v1alpha2.VirtualImage
			viPvcFromVD                         *v1alpha2.VirtualImage
			viPvcFromVDSnapshot                 *v1alpha2.VirtualImage
			cviFromVIHttp                       *v1alpha2.ClusterVirtualImage
			cviFromVIContainerImage             *v1alpha2.ClusterVirtualImage
			cviFromVIFromCVIHttp                *v1alpha2.ClusterVirtualImage
			cviFromVIFromCVIContainerImage      *v1alpha2.ClusterVirtualImage
			cviFromVIFromVD                     *v1alpha2.ClusterVirtualImage
			cviFromVIFromVDSnapshot             *v1alpha2.ClusterVirtualImage
			cviFromVIPVCFromHttp                *v1alpha2.ClusterVirtualImage
			cviFromVIPVCFromContainerImage      *v1alpha2.ClusterVirtualImage
			cviFromVIPVCFromVD                  *v1alpha2.ClusterVirtualImage
			cviFromVIPVCFromVDSnapshot          *v1alpha2.ClusterVirtualImage
			cviFromVIPVCFromCVIHttp             *v1alpha2.ClusterVirtualImage
			cviFromVIPVCFromCVIContainerImage   *v1alpha2.ClusterVirtualImage
			viFromVIHttp                        *v1alpha2.VirtualImage
			viFromVIContainerImage              *v1alpha2.VirtualImage
			viFromVIFromCVIHttp                 *v1alpha2.VirtualImage
			viFromVIFromCVIContainerImage       *v1alpha2.VirtualImage
			viFromVIFromVD                      *v1alpha2.VirtualImage
			viFromVIFromVDSnapshot              *v1alpha2.VirtualImage
			viFromVIPVCFromHttp                 *v1alpha2.VirtualImage
			viFromVIPVCFromContainerImage       *v1alpha2.VirtualImage
			viFromVIPVCFromVD                   *v1alpha2.VirtualImage
			viFromVIPVCFromVDSnapshot           *v1alpha2.VirtualImage
			viFromVIPVCFromCVIHttp              *v1alpha2.VirtualImage
			viFromVIPVCFromCVIContainerImage    *v1alpha2.VirtualImage
			viPVCFromVIHttp                     *v1alpha2.VirtualImage
			viPVCFromVIContainerImage           *v1alpha2.VirtualImage
			viPVCFromVIFromCVIHttp              *v1alpha2.VirtualImage
			viPVCFromVIFromCVIContainerImage    *v1alpha2.VirtualImage
			viPVCFromVIFromVD                   *v1alpha2.VirtualImage
			viPVCFromVIFromVDSnapshot           *v1alpha2.VirtualImage
			viPVCFromVIPVCFromHttp              *v1alpha2.VirtualImage
			viPVCFromVIPVCFromContainerImage    *v1alpha2.VirtualImage
			viPVCFromVIPVCFromVD                *v1alpha2.VirtualImage
			viPVCFromVIPVCFromVDSnapshot        *v1alpha2.VirtualImage
			viPVCFromVIPVCFromCVIHttp           *v1alpha2.VirtualImage
			viPVCFromVIPVCFromCVIContainerImage *v1alpha2.VirtualImage
		)

		By("Creating VirtualDisk", func() {
			vd = object.NewGeneratedHTTPVDUbuntu("vd-", f.Namespace().Name)
			err := f.CreateWithDeferredDeletion(context.Background(), vd)
			Expect(err).NotTo(HaveOccurred())
			vm := object.NewMinimalVM("vm-", f.Namespace().Name, vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}))
			err = f.CreateWithDeferredDeletion(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(vd, string(v1alpha2.DiskReady), framework.LongTimeout)
			err = f.Delete(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualDiskSnapshot", func() {
			vdSnapshot = vdsnapshotbuilder.New(
				vdsnapshotbuilder.WithGenerateName("vdsnapshot-"),
				vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vdsnapshotbuilder.WithVirtualDiskName(vd.Name),
				vdsnapshotbuilder.WithRequiredConsistency(true),
			)
			err := f.CreateWithDeferredDeletion(context.Background(), vdSnapshot)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(vdSnapshot, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.ShortTimeout)
		})

		By("Creating images", func() {
			cviHttp = object.NewGenerateHTTPCVIUbuntu(fmt.Sprintf("%s-cvi-http-", cviPrefix))
			err := f.CreateWithDeferredDeletion(context.Background(), cviHttp)
			Expect(err).NotTo(HaveOccurred())

			cviContainerImage = object.NewGenerateContainerImageCVI(fmt.Sprintf("%s-cvi-ci-", cviPrefix))
			err = f.CreateWithDeferredDeletion(context.Background(), cviContainerImage)
			Expect(err).NotTo(HaveOccurred())

			cviFromCVIHttp = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-cvi-http-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name, ""),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			cviFromCVIContainerImage = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-cvi-ci-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name, ""),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			cviFromVD = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vd-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk, vd.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVD)
			Expect(err).NotTo(HaveOccurred())

			cviFromVDSnapshot = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vds-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			viHttp = object.NewGeneratedHTTPVIUbuntu("vi-http-", f.Namespace().Name)
			err = f.CreateWithDeferredDeletion(context.Background(), viHttp)
			Expect(err).NotTo(HaveOccurred())

			viContainerImage = object.NewGeneratedContainerImageVI("vi-ci-", f.Namespace().Name)
			err = f.CreateWithDeferredDeletion(context.Background(), viContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viFromCVIHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-cvi-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			viFromCVIContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-cvi-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viFromVD = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vd-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVD)
			Expect(err).NotTo(HaveOccurred())

			viFromVDSnapshot = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vds-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			viPvcHttp = object.NewGeneratedHTTPVIUbuntu("vi-pvc-http-", f.Namespace().Name, vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim))
			err = f.CreateWithDeferredDeletion(context.Background(), viPvcHttp)
			Expect(err).NotTo(HaveOccurred())

			viPvcContainerImage = object.NewGeneratedContainerImageVI("vi-pvc-ci-", f.Namespace().Name, vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim))
			err = f.CreateWithDeferredDeletion(context.Background(), viPvcContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viPvcFromCVIHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-cvi-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPvcFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			viPvcFromCVIContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-cvi-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPvcFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viPvcFromVD = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vd-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPvcFromVD)
			Expect(err).NotTo(HaveOccurred())

			viPvcFromVDSnapshot = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vds-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPvcFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIHttp = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-http-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage, viHttp.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIHttp)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIContainerImage = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-ci-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage, viContainerImage.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIFromCVIHttp = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-from-cvi-http-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIFromCVIContainerImage = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-from-cvi-ci-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIFromVD = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-from-vd-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk, vd.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIFromVD)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIFromVDSnapshot = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-from-vds-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIPVCFromHttp = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-pvc-http-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage, viPvcHttp.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIPVCFromHttp)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIPVCFromContainerImage = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-pvc-ci-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage, viPvcContainerImage.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIPVCFromContainerImage)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIPVCFromVD = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-pvc-vd-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk, vd.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIPVCFromVD)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIPVCFromVDSnapshot = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-pvc-vds-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIPVCFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIPVCFromCVIHttp = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-pvc-cvi-http-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIPVCFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			cviFromVIPVCFromCVIContainerImage = cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vi-pvc-cvi-ci-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name, f.Namespace().Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), cviFromVIPVCFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viFromVIHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIHttp)
			Expect(err).NotTo(HaveOccurred())

			viFromVIContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viFromVIFromCVIHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-from-cvi-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			viFromVIFromCVIContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-from-cvi-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viFromVIFromVD = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-from-vd-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIFromVD)
			Expect(err).NotTo(HaveOccurred())

			viFromVIFromVDSnapshot = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-from-vds-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			viFromVIPVCFromHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-pvc-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, viPvcHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIPVCFromHttp)
			Expect(err).NotTo(HaveOccurred())

			viFromVIPVCFromContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-pvc-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, viPvcContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIPVCFromContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viFromVIPVCFromVD = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-pvc-vd-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIPVCFromVD)
			Expect(err).NotTo(HaveOccurred())

			viFromVIPVCFromVDSnapshot = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-pvc-vds-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIPVCFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			viFromVIPVCFromCVIHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-pvc-cvi-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIPVCFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			viFromVIPVCFromCVIContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vi-pvc-cvi-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viFromVIPVCFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIHttp = object.NewGeneratedHTTPVIUbuntu("vi-pvc-from-vi-http-", f.Namespace().Name, vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim))
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIHttp)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIContainerImage = object.NewGeneratedContainerImageVI("vi-pvc-from-vi-ci-", f.Namespace().Name, vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim))
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIFromCVIHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-from-cvi-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIFromCVIContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-from-cvi-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIFromVD = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-from-vd-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIFromVD)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIFromVDSnapshot = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-from-vds-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIPVCFromHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-pvc-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, viPvcHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIPVCFromHttp)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIPVCFromContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-pvc-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, viPvcContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIPVCFromContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIPVCFromVD = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-pvc-vd-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIPVCFromVD)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIPVCFromVDSnapshot = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-pvc-vds-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIPVCFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIPVCFromCVIHttp = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-pvc-cvi-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviHttp.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIPVCFromCVIHttp)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIPVCFromCVIContainerImage = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-pvc-cvi-ci-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, cviContainerImage.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIPVCFromCVIContainerImage)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIPVCFromVD = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-pvc-vd-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIPVCFromVD)
			Expect(err).NotTo(HaveOccurred())

			viPVCFromVIPVCFromVDSnapshot = vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vi-pvc-vds-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			err = f.CreateWithDeferredDeletion(context.Background(), viPVCFromVIPVCFromVDSnapshot)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Verifying that images are ready", func() {
			Eventually(func(g Gomega) {
				err := f.UpdateFromCluster(
					context.Background(),
					cviHttp,
					cviContainerImage,
					cviFromCVIHttp,
					cviFromCVIContainerImage,
					cviFromVD,
					cviFromVDSnapshot,
					viHttp,
					viContainerImage,
					viFromCVIHttp,
					viFromCVIContainerImage,
					viFromVD,
					viFromVDSnapshot,
					viPvcHttp,
					viPvcContainerImage,
					viPvcFromCVIHttp,
					viPvcFromCVIContainerImage,
					viPvcFromVD,
					viPvcFromVDSnapshot,
					cviFromVIHttp,
					cviFromVIContainerImage,
					cviFromVIFromCVIHttp,
					cviFromVIFromCVIContainerImage,
					cviFromVIFromVD,
					cviFromVIFromVDSnapshot,
					cviFromVIPVCFromHttp,
					cviFromVIPVCFromContainerImage,
					cviFromVIPVCFromVD,
					cviFromVIPVCFromVDSnapshot,
					cviFromVIPVCFromCVIHttp,
					cviFromVIPVCFromCVIContainerImage,
					viFromVIHttp,
					viFromVIContainerImage,
					viFromVIFromCVIHttp,
					viFromVIFromCVIContainerImage,
					viFromVIFromVD,
					viFromVIFromVDSnapshot,
					viFromVIPVCFromHttp,
					viFromVIPVCFromContainerImage,
					viFromVIPVCFromVD,
					viFromVIPVCFromVDSnapshot,
					viFromVIPVCFromCVIHttp,
					viFromVIPVCFromCVIContainerImage,
					viPVCFromVIHttp,
					viPVCFromVIContainerImage,
					viPVCFromVIFromCVIHttp,
					viPVCFromVIFromCVIContainerImage,
					viPVCFromVIFromVD,
					viPVCFromVIFromVDSnapshot,
					viPVCFromVIPVCFromHttp,
					viPVCFromVIPVCFromContainerImage,
					viPVCFromVIPVCFromVD,
					viPVCFromVIPVCFromVDSnapshot,
					viPVCFromVIPVCFromCVIHttp,
					viPVCFromVIPVCFromCVIContainerImage,
				)
				Expect(err).NotTo(HaveOccurred())

				g.Expect(cviHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(cviContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(cviFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(cviFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(cviFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(cviFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPvcHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPvcContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPvcFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPvcFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPvcFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPvcFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIPVCFromHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIPVCFromContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIPVCFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIPVCFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIPVCFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viFromVIPVCFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromVD.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromVDSnapshot.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromCVIHttp.Status.Phase).To(Equal(v1alpha2.ImageReady))
				g.Expect(viPVCFromVIPVCFromCVIContainerImage.Status.Phase).To(Equal(v1alpha2.ImageReady))
			}, framework.LongTimeout, time.Second).Should(Succeed())
		})
	})
})
