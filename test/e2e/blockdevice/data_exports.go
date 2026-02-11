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
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	mountPointData       = "/mnt"
	fileDataPath         = "/mnt/testfile"
	testFileValue        = "test-file-value"
	exportedDiskFile     = "exported-disk.img"
	exportedSnapshotFile = "exported-snapshot.img"
)

var _ = Describe("DataExports", label.Slow(), func() {
	f := framework.NewFramework("data-exports")

	BeforeEach(func() {
		moduleEnabled, err := checkStorageVolumeDataManagerEnabled()
		Expect(err).NotTo(HaveOccurred(), "Failed to get modules")
		if !moduleEnabled {
			Skip("Module 'storage-volume-data-manager' is disabled. Skipping all tests with using this module.")
		}

		f.Before()
		DeferCleanup(f.After)
	})

	It("exports VirtualDisk and VirtualDiskSnapshot, then restores data via upload", func() {
		var (
			vdRoot               *v1alpha2.VirtualDisk
			vdData               *v1alpha2.VirtualDisk
			vdSnapshot           *v1alpha2.VirtualDiskSnapshot
			vdFromDiskExport     *v1alpha2.VirtualDisk
			vdFromSnapshotExport *v1alpha2.VirtualDisk
			vm                   *v1alpha2.VirtualMachine
		)

		By("Creating root and data disks", func() {
			vdRoot = vdbuilder.New(
				vdbuilder.WithName("vd-root"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLUbuntu,
				}),
			)

			vdData = vdbuilder.New(
				vdbuilder.WithName("vd-data"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("51Mi"))),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vdRoot, vdData)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualMachine", func() {
			vm = vmbuilder.New(
				vmbuilder.WithName("vm"),
				vmbuilder.WithNamespace(f.Namespace().Name),
				vmbuilder.WithCPU(1, ptr.To("5%")),
				vmbuilder.WithMemory(resource.MustParse("256Mi")),
				vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
				vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: vdRoot.Name},
					v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: vdData.Name},
				),
				vmbuilder.WithRunPolicy(v1alpha2.AlwaysOnUnlessStoppedManually),
				vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for VM agent to be ready", func() {
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Writing test data to the data disk", func() {
			util.CreateBlockDeviceFilesystem(f, vm, v1alpha2.DiskDevice, vdData.Name, "ext4")
			util.MountBlockDevice(f, vm, v1alpha2.DiskDevice, vdData.Name, mountPointData)
			util.WriteFile(f, vm, fileDataPath, testFileValue)
			util.UnmountBlockDevice(f, vm, mountPointData)
		})

		By("Stopping the VM", func() {
			vmopStop := vmopbuilder.New(
				vmopbuilder.WithGenerateName(fmt.Sprintf("%s-stop-", util.VmopE2ePrefix)),
				vmopbuilder.WithNamespace(vm.Namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
				vmopbuilder.WithVirtualMachine(vm.Name),
			)
			err := f.CreateWithDeferredDeletion(context.Background(), vmopStop)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, vmopStop)
			util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.ShortTimeout, vm)
		})

		By("Creating snapshot of the data disk", func() {
			vdSnapshot = vdsnapshotbuilder.New(
				vdsnapshotbuilder.WithName("vdsnapshot-data"),
				vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vdsnapshotbuilder.WithVirtualDiskName(vdData.Name),
				vdsnapshotbuilder.WithRequiredConsistency(true),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vdSnapshot)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.ShortTimeout, vdSnapshot)
		})

		By("Exporting VirtualDisk to local file", func() {
			exportData(f, "vd", vdData.Name, exportedDiskFile)
		})

		By("Exporting VirtualDiskSnapshot to local file", func() {
			exportData(f, "vds", vdSnapshot.Name, exportedSnapshotFile)
		})

		By("Deleting the original data disk", func() {
			err := f.Delete(context.Background(), vdData)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				var vd v1alpha2.VirtualDisk
				err := f.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
					Namespace: vdData.Namespace,
					Name:      vdData.Name,
				}, &vd)
				g.Expect(crclient.IgnoreNotFound(err)).NotTo(HaveOccurred())
				g.Expect(err).To(HaveOccurred(), "VirtualDisk should be deleted")
			}, framework.MiddleTimeout, time.Second).Should(Succeed())
		})

		By("Creating disk from exported VirtualDisk", func() {
			vdFromDiskExport = createUploadDisk(f, "vd-restored-from-disk")
		})

		By("Uploading exported disk image", func() {
			uploadFile(f, vdFromDiskExport, exportedDiskFile)
		})

		By("Waiting for disk from VirtualDisk export to be ready", func() {
			util.UntilObjectPhase(util.GetExpectedDiskPhaseByVolumeBindingMode(), framework.LongTimeout, vdFromDiskExport)
		})

		By("Creating disk from exported VirtualDiskSnapshot", func() {
			vdFromSnapshotExport = createUploadDisk(f, "vd-restored-from-snapshot")
		})

		By("Uploading exported snapshot image", func() {
			uploadFile(f, vdFromSnapshotExport, exportedSnapshotFile)
		})

		By("Waiting for disk from snapshot export to be ready", func() {
			util.UntilObjectPhase(util.GetExpectedDiskPhaseByVolumeBindingMode(), framework.LongTimeout, vdFromSnapshotExport)
		})

		By("Attaching restored disks to VM", func() {
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
			Expect(err).NotTo(HaveOccurred())

			vm.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: vdRoot.Name},
				{Kind: v1alpha2.DiskDevice, Name: vdFromDiskExport.Name},
				{Kind: v1alpha2.DiskDevice, Name: vdFromSnapshotExport.Name},
			}

			err = f.Clients.GenericClient().Update(context.Background(), vm)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Starting the VM", func() {
			util.StartVirtualMachine(f, vm)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Verifying data on disk restored from VirtualDisk export", func() {
			util.MountBlockDevice(f, vm, v1alpha2.DiskDevice, vdFromDiskExport.Name, mountPointData)
			restoredValue := util.ReadFile(f, vm, fileDataPath)
			Expect(restoredValue).To(Equal(testFileValue), "Data should match original")
			util.UnmountBlockDevice(f, vm, mountPointData)
		})

		By("Verifying data on disk restored from VirtualDiskSnapshot export", func() {
			util.MountBlockDevice(f, vm, v1alpha2.DiskDevice, vdFromSnapshotExport.Name, mountPointData)
			restoredValue := util.ReadFile(f, vm, fileDataPath)
			Expect(restoredValue).To(Equal(testFileValue), "Data should match original")
			util.UnmountBlockDevice(f, vm, mountPointData)
		})
	})
})

func needPublishOption(f *framework.Framework) bool {
	hostname, err := os.Hostname()
	Expect(err).NotTo(HaveOccurred(), "Failed to get hostname")
	var node corev1.Node
	err = f.Clients.GenericClient().Get(
		context.Background(),
		types.NamespacedName{Name: hostname},
		&node,
	)
	if k8serrors.IsNotFound(err) {
		return true
	}
	Expect(err).NotTo(HaveOccurred(), "Failed to get node %s", hostname)
	return false
}

func exportData(f *framework.Framework, resourceType, name, outputFile string) {
	result := f.D8Virtualization().DataExportDownload(resourceType, name, d8.DataExportOptions{
		Namespace:  f.Namespace().Name,
		OutputFile: outputFile,
		Publish:    needPublishOption(f),
		Timeout:    framework.LongTimeout,
	})
	Expect(result.WasSuccess()).To(BeTrue(), "d8 data export download failed: %s", result.StdErr())

	DeferCleanup(func() {
		err := os.Remove(outputFile)
		Expect(err == nil || errors.Is(err, os.ErrNotExist)).To(BeTrue(), "Failed to remove exported file %s: %v", outputFile, err)
	})
}

func createUploadDisk(f *framework.Framework, name string) *v1alpha2.VirtualDisk {
	vd := vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(f.Namespace().Name),
		vdbuilder.WithDatasource(&v1alpha2.VirtualDiskDataSource{
			Type: v1alpha2.DataSourceTypeUpload,
		}),
	)

	err := f.CreateWithDeferredDeletion(context.Background(), vd)
	Expect(err).NotTo(HaveOccurred())
	util.UntilObjectPhase(string(v1alpha2.DiskWaitForUserUpload), framework.LongTimeout, vd)

	return vd
}

func retry(maxRetries int, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			GinkgoWriter.Printf("Attempt %d/%d failed: %v\n", attempt, maxRetries, err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		return nil
	}
	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func uploadFile(f *framework.Framework, vd *v1alpha2.VirtualDisk, filePath string) {
	err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vd), vd)
	Expect(err).NotTo(HaveOccurred())
	Expect(vd.Status.ImageUploadURLs).NotTo(BeNil(), "ImageUploadURLs should be set")
	Expect(vd.Status.ImageUploadURLs.External).NotTo(BeEmpty(), "External upload URL should be set")

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	uploadURL := vd.Status.ImageUploadURLs.External

	// During the upload of a VirtualDisk of type 'Upload', there is a bug:
	//  when the VirtualDisk is in the 'DiskWaitForUserUpload' phase,
	//  nginx may not be ready yet and can return 413 or 503 errors.
	//  Once this bug is fixed, the retry mechanism must be removed.
	const maxRetries = 5
	err = retry(maxRetries, func() error {
		return doUploadAttempt(httpClient, uploadURL, filePath)
	})
	Expect(err).NotTo(HaveOccurred(), "Upload failed")
}

func doUploadAttempt(client *http.Client, url, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "Failed to close file %s", filePath)
		}
	}()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", filePath, err)
	}
	if stat.Size() == 0 {
		return fmt.Errorf("file %s is empty", filePath)
	}

	req, err := http.NewRequest(http.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.ContentLength = stat.Size()

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "Failed to close response body")
		}
	}()

	return handleUploadResponse(resp)
}

func handleUploadResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, body)
}

func checkStorageVolumeDataManagerEnabled() (bool, error) {
	sdnModule, err := framework.NewFramework("").GetModuleConfig("storage-volume-data-manager")
	if err != nil {
		return false, err
	}
	enabled := sdnModule.Spec.Enabled

	return enabled != nil && *enabled, nil
}
