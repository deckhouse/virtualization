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
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
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
			vd         *v1alpha2.VirtualDisk
			vdSnapshot *v1alpha2.VirtualDiskSnapshot
			vis        []*v1alpha2.VirtualImage
			cvis       []*v1alpha2.ClusterVirtualImage
		)

		By("Creating VirtualDisk", func() {
			vd = vdbuilder.New(
				vdbuilder.WithGenerateName("vd-"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceHTTP(
					&v1alpha2.DataSourceHTTP{
						URL: object.ImageURLAlpineUEFIPerf,
					},
				),
			)
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
			var (
				baseCvis []*v1alpha2.ClusterVirtualImage
				baseVis  []*v1alpha2.VirtualImage
			)

			baseCvis = append(baseCvis, object.NewGenerateContainerImageCVI(fmt.Sprintf("%s-cvi-ci-", cviPrefix)))
			baseCvis = append(baseCvis, cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-http-", cviPrefix)),
				cvibuilder.WithDataSourceHTTP(
					object.ImageURLAlpineUEFIPerf,
					nil,
					nil,
				),
			))
			baseCvis = append(baseCvis, cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vd-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDisk, vd.Name, f.Namespace().Name),
			))
			baseCvis = append(baseCvis, cvibuilder.New(
				cvibuilder.WithGenerateName(fmt.Sprintf("%s-cvi-from-vds-", cviPrefix)),
				cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name, f.Namespace().Name),
			))
			for _, cvi := range baseCvis {
				err := f.CreateWithDeferredDeletion(context.Background(), cvi)
				Expect(err).NotTo(HaveOccurred())
			}

			baseVis = append(baseVis, object.NewGeneratedContainerImageVI("vi-ci-", f.Namespace().Name))
			baseVis = append(baseVis, vibuilder.New(
				vibuilder.WithGenerateName("vi-http-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceHTTP(
					object.ImageURLAlpineUEFIPerf,
					nil,
					nil,
				),
			))
			baseVis = append(baseVis, vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vd-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			))
			baseVis = append(baseVis, vibuilder.New(
				vibuilder.WithGenerateName("vi-from-vds-"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			))
			baseVis = append(baseVis, object.NewGeneratedHTTPVIUbuntu("vi-pvc-http-", f.Namespace().Name, vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim)))
			baseVis = append(baseVis, object.NewGeneratedContainerImageVI("vi-pvc-ci-", f.Namespace().Name, vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim)))
			baseVis = append(baseVis, vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vd-"),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			))
			baseVis = append(baseVis, vibuilder.New(
				vibuilder.WithGenerateName("vi-pvc-from-vds-"),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			))
			for _, vi := range baseVis {
				err := f.CreateWithDeferredDeletion(context.Background(), vi)
				Expect(err).NotTo(HaveOccurred())
			}

			// Create cluster virtual images from cluster virtual images
			for _, baseCvi := range baseCvis {
				cvis = append(cvis, cvibuilder.New(
					cvibuilder.WithName(fmt.Sprintf("%s-cvi-from-%s", cviPrefix, baseCvi.Name)),
					cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage, baseCvi.Name, ""),
				))
			}

			// Create cluster virtual images from virtual images
			for _, baseVi := range baseVis {
				cvis = append(cvis, cvibuilder.New(
					cvibuilder.WithName(fmt.Sprintf("%s-cvi-from-%s", cviPrefix, baseVi.Name)),
					cvibuilder.WithDataSourceObjectRef(v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage, baseVi.Name, baseVi.Namespace),
				))
			}

			// Create virtual images from cluster virtual images
			for _, baseCvi := range baseCvis {
				vis = append(vis, vibuilder.New(
					vibuilder.WithName(fmt.Sprintf("vi-from-%s", baseCvi.Name)),
					vibuilder.WithNamespace(f.Namespace().Name),
					vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, baseCvi.Name),
					vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
				))
			}

			// Create virtual images from virtual images
			for _, baseVi := range baseVis {
				vis = append(vis, vibuilder.New(
					vibuilder.WithName(fmt.Sprintf("vi-from-%s", baseVi.Name)),
					vibuilder.WithNamespace(f.Namespace().Name),
					vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
					vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVi.Name),
				))
			}

			// Create pvc virtual images from cluster virtual images
			for _, baseCvi := range baseCvis {
				vis = append(vis, vibuilder.New(
					vibuilder.WithName(fmt.Sprintf("vi-pvc-from-%s", baseCvi.Name)),
					vibuilder.WithNamespace(f.Namespace().Name),
					vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, baseCvi.Name),
					vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				))
			}

			// Create pvc virtual images from virtual images
			for _, baseVi := range baseVis {
				vis = append(vis, vibuilder.New(
					vibuilder.WithName(fmt.Sprintf("vi-pvc-from-%s", baseVi.Name)),
					vibuilder.WithNamespace(f.Namespace().Name),
					vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
					vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVi.Name),
				))
			}

			for _, vi := range vis {
				err := f.CreateWithDeferredDeletion(context.Background(), vi)
				Expect(err).NotTo(HaveOccurred())
			}

			for _, cvi := range cvis {
				err := f.CreateWithDeferredDeletion(context.Background(), cvi)
				Expect(err).NotTo(HaveOccurred())
			}

			// All entities created in the previous steps must be taken into account
			vis = append(vis, baseVis...)
			cvis = append(cvis, baseCvis...)
		})

		By("Verifying that images are ready", func() {
			Eventually(func(g Gomega) {
				for _, vi := range vis {
					f.UpdateFromCluster(context.Background(), vi)
					g.Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageReady))
				}
				for _, cvi := range cvis {
					f.UpdateFromCluster(context.Background(), cvi)
					g.Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageReady))
				}
			}, framework.LongTimeout, time.Second).Should(Succeed())
		})
	})
})
