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

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	projobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/project"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("ImporterNetworkPolicy", Label(label.SIGStorage, precheck.NoPrecheck), func() {
	const testName = "importer-network-policy"

	var (
		f   *framework.Framework
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
	})

	It("test network policy isolation for vi importer", func() {
		By("Create isolated project")
		project := object.NewIsolatedProject(testName, framework.NamespaceBasePrefix)
		err := f.CreateWithDeferredDeletion(ctx, project)
		Expect(err).NotTo(HaveOccurred())
		projObs := projobs.StartObserver(ctx, f, project.Name)
		err = projObs.WaitFor(projobs.BeDeployed(), framework.ShortTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Create virtual image")
		vi := object.NewGeneratedHTTPVICustomBIOS("vi-", project.Name)
		err = f.CreateWithDeferredDeletion(ctx, vi)
		Expect(err).NotTo(HaveOccurred())

		By("Check VI reaches the Ready phase", func() {
			viObs := viobs.StartObserver(ctx, f, vi)
			viObs.Never(viobs.BeFailed())
			viObs.Always(viobs.HaveValidPhaseTransitions())
			// The controller scrapes progress metrics from the importer pod over the
			// network; in an isolated project that scrape is blocked unless the
			// importer CiliumNetworkPolicy allows ingress from the controller
			// namespace, so an intermediate progress value is a strong signal that
			// the metrics path works here.
			viObs.Always(viobs.HaveValidProgress(virtualImageProgressExpectations(vi, progressWaitOptions{})))
			err := viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("test network policy isolation for vd importer", func() {
		By("Create isolated project")
		project := object.NewIsolatedProject(testName, framework.NamespaceBasePrefix)
		err := f.CreateWithDeferredDeletion(ctx, project)
		Expect(err).NotTo(HaveOccurred())
		projObs := projobs.StartObserver(ctx, f, project.Name)
		err = projObs.WaitFor(projobs.BeDeployed(), framework.ShortTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Create virtual disk")
		vd := object.NewHTTPVDCustomBIOS("vd", project.Name, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))))
		err = f.CreateWithDeferredDeletion(ctx, vd)
		Expect(err).NotTo(HaveOccurred())

		By("Create virtual machine")
		// The custom image has no cloud-init; this VM is only the disk
		// consumer that unparks a WaitForFirstConsumer disk, so provision nothing.
		vm := object.NewMinimalVM("vm-", project.Name, vmbuilder.WithDisks(vd))
		err = f.CreateWithDeferredDeletion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		By("Check VD reaches the Ready phase", func() {
			vdObs := vdobs.StartObserver(ctx, f, vd)
			vdObs.Never(vdobs.BeFailed())
			err := vdObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("test network policy isolation for vi upload", func() {
		By("Create isolated project")
		project := object.NewIsolatedProject(testName, framework.NamespaceBasePrefix)
		err := f.CreateWithDeferredDeletion(ctx, project)
		Expect(err).NotTo(HaveOccurred())
		projObs := projobs.StartObserver(ctx, f, project.Name)
		err = projObs.WaitFor(projobs.BeDeployed(), framework.ShortTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Download source image to upload")
		uploadFilePath, err := downloadImageToTempFile(object.ImageURLCustomBIOS)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			removeErr := os.Remove(uploadFilePath)
			Expect(removeErr == nil || errors.Is(removeErr, os.ErrNotExist)).To(BeTrue(),
				"failed to remove upload source file %q: %v", uploadFilePath, removeErr)
		})

		By("Create virtual image with Upload data source")
		vi := object.NewVI(
			vibuilder.WithGenerateName("vi-upload-"),
			vibuilder.WithNamespace(project.Name),
			vibuilder.WithDatasource(v1alpha2.VirtualImageDataSource{
				Type: v1alpha2.DataSourceTypeUpload,
			}),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
		)
		err = f.CreateWithDeferredDeletion(ctx, vi)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the VirtualImage to expose upload URLs", func() {
			viObs := viobs.StartObserver(ctx, f, vi)
			viObs.Never(viobs.BeFailed())
			err := viObs.WaitFor(viobs.BeReadyForUserUpload(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Uploading data to the VirtualImage", func() {
			// Re-fetch to read the upload URL published in status.
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vi), vi)
			Expect(err).NotTo(HaveOccurred())
			Expect(vi.Status.ImageUploadURLs).NotTo(BeNil())
			Expect(vi.Status.ImageUploadURLs.External).NotTo(BeEmpty())
			err = doRetriableUploadAttempt(vi.Status.ImageUploadURLs.External, uploadFilePath)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Check VI reaches the Ready phase", func() {
			viObs := viobs.StartObserver(ctx, f, vi)
			viObs.Never(viobs.BeFailed())
			err := viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("test network policy isolation for vd upload", func() {
		By("Create isolated project")
		project := object.NewIsolatedProject(testName, framework.NamespaceBasePrefix)
		err := f.CreateWithDeferredDeletion(ctx, project)
		Expect(err).NotTo(HaveOccurred())
		projObs := projobs.StartObserver(ctx, f, project.Name)
		err = projObs.WaitFor(projobs.BeDeployed(), framework.ShortTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Download source image to upload")
		uploadFilePath, err := downloadImageToTempFile(object.ImageURLCustomBIOS)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			removeErr := os.Remove(uploadFilePath)
			Expect(removeErr == nil || errors.Is(removeErr, os.ErrNotExist)).To(BeTrue(),
				"failed to remove upload source file %q: %v", uploadFilePath, removeErr)
		})

		By("Create virtual disk with Upload data source")
		vd := object.NewVD(
			vdbuilder.WithGenerateName("vd-upload-"),
			vdbuilder.WithNamespace(project.Name),
			vdbuilder.WithPersistentVolumeClaim(defaultStorageClass(), ptr.To(resource.MustParse(vdCreationImageSize))),
			vdbuilder.WithDatasource(&v1alpha2.VirtualDiskDataSource{
				Type: v1alpha2.DataSourceTypeUpload,
			}),
		)
		err = f.CreateWithDeferredDeletion(ctx, vd)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for the VirtualDisk to expose upload URLs", func() {
			vdObs := vdobs.StartObserver(ctx, f, vd)
			vdObs.Never(vdobs.BeFailed())
			err := vdObs.WaitFor(vdobs.BeReadyForUserUpload(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Uploading data to the VirtualDisk", func() {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vd), vd)
			Expect(err).NotTo(HaveOccurred())
			Expect(vd.Status.ImageUploadURLs).NotTo(BeNil())
			Expect(vd.Status.ImageUploadURLs.External).NotTo(BeEmpty())
			err = doRetriableUploadAttempt(vd.Status.ImageUploadURLs.External, uploadFilePath)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Create virtual machine")
		// On a WaitForFirstConsumer storage class the uploaded data lands in DVCR,
		// and the final import into the disk's volume only runs once the disk has a
		// consumer. The custom image has no cloud-init; this VM is only that consumer,
		// so provision nothing.
		vm := object.NewMinimalVM("vm-", project.Name, vmbuilder.WithDisks(vd))
		err = f.CreateWithDeferredDeletion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		By("Check VD reaches the Ready phase", func() {
			vdObs := vdobs.StartObserver(ctx, f, vd)
			vdObs.Never(vdobs.BeFailed())
			err := vdObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
