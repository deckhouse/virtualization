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
	"path/filepath"
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
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	mountPointData       = "/mnt"
	fileDataPath         = "/mnt/testfile"
	testFileValue        = "test-file-value"
	exportedDiskFile     = "exported-disk.img"
	exportedSnapshotFile = "exported-snapshot.img"
	// diskImageExportFile is the filename inside a filesystem-backed (e.g. NFS) volume export.
	diskImageExportFile = "disk.img"
)

var _ = label.SIGDescribe(label.SIGStorage, "DataExports", label.Slow(), Label(precheck.PrecheckSVDM, precheck.PrecheckSnapshot), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("data-exports")

		f.Before()
		DeferCleanup(f.After)
	})

	It("exports VirtualDisk and VirtualDiskSnapshot, then restores data via upload", func() {
		// Data export downloads the disk bytes from an in-cluster exporter. Off
		// cluster (e.g. running the suite over a kube-apiserver tunnel from a
		// laptop) d8 must fall back to publish mode. In principle publish mode
		// should still work, but it is currently broken by a bug in the export
		// module (storage-volume-data-manager): its publish path looks up the
		// origin Ingress at the hard-coded location "d8-user-authn/kubernetes-api",
		// while on current Deckhouse that Ingress is created by control-plane-manager
		// in "kube-system", so the export fails with PublishFailed.
		//
		// TODO: this skip is a workaround for that export-module bug. Remove it once
		// storage-volume-data-manager resolves the origin-Ingress lookup (e.g. makes
		// the namespace configurable or also searches kube-system), so the test runs
		// off-cluster too. Until then the test still runs on a cluster node / in CI,
		// where the in-cluster download path needs no publish.
		if !runningOnClusterNode(ctx, f) {
			Skip("data export requires the suite to run on a cluster node (in-cluster download); skipped off-cluster due to a publish-mode bug in the storage-volume-data-manager export module")
		}

		var (
			vdRoot               *v1alpha2.VirtualDisk
			vdData               *v1alpha2.VirtualDisk
			vdSnapshot           *v1alpha2.VirtualDiskSnapshot
			vdFromDiskExport     *v1alpha2.VirtualDisk
			vdFromSnapshotExport *v1alpha2.VirtualDisk
			vm                   *v1alpha2.VirtualMachine
		)

		// Export downloads go to a per-spec temp dir (auto-removed by Ginkgo), not
		// the working directory, so a run never leaves artifacts in the repo.
		exportDir := GinkgoT().TempDir()
		diskExportPath := filepath.Join(exportDir, exportedDiskFile)
		snapshotExportPath := filepath.Join(exportDir, exportedSnapshotFile)

		By("Creating root and data disks", func() {
			vdRoot = object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS,
				vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))),
				vdbuilder.WithStorageClass(defaultStorageClass()))

			vdData = vdbuilder.New(
				vdbuilder.WithName("vd-data"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithPersistentVolumeClaim(defaultStorageClass(), ptr.To(resource.MustParse(vdCreationImageSize))),
			)

			err := f.CreateWithDeferredDeletion(ctx, vdRoot, vdData)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualMachine", func() {
			vm = vmbuilder.New(
				vmbuilder.WithName("vm"),
				vmbuilder.WithNamespace(f.Namespace().Name),
				vmbuilder.WithCPU(1, ptr.To("100%")),
				vmbuilder.WithMemory(resource.MustParse("512Mi")),
				vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
				vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: vdRoot.Name},
					v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: vdData.Name},
				),
				vmbuilder.WithRunPolicy(v1alpha2.AlwaysOnUnlessStoppedManually),
			)

			err := f.CreateWithDeferredDeletion(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
		})

		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())

		By("Waiting for VM agent to be ready", func() {
			Expect(vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)).To(Succeed())
		})

		By("Writing test data to the data disk", func() {
			guestCreateFilesystem(ctx, f, vm, v1alpha2.DiskDevice, vdData.Name, "ext4")
			guestMount(ctx, f, vm, v1alpha2.DiskDevice, vdData.Name, mountPointData)
			guestWriteFile(f, vm, fileDataPath, testFileValue)
			guestUnmount(f, vm, mountPointData)
		})

		By("Stopping the VM", func() {
			vmopStop := vmopbuilder.New(
				vmopbuilder.WithGenerateName(fmt.Sprintf("%s-stop-", util.VmopE2ePrefix)),
				vmopbuilder.WithNamespace(vm.Namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
				vmopbuilder.WithVirtualMachine(vm.Name),
			)
			err := f.CreateWithDeferredDeletion(ctx, vmopStop)
			Expect(err).NotTo(HaveOccurred())

			vmopObs := vmopobs.StartObserver(ctx, vmopStop)
			Expect(vmopObs.WaitFor(vmopobs.BeCompleted(), framework.LongTimeout)).To(Succeed())
			Expect(vmObs.WaitFor(vmobs.BeStopped(), framework.ShortTimeout)).To(Succeed())
		})

		By("Creating snapshot of the data disk", func() {
			vdSnapshot = vdsnapshotbuilder.New(
				vdsnapshotbuilder.WithName("vdsnapshot-data"),
				vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vdsnapshotbuilder.WithVirtualDiskName(vdData.Name),
				vdsnapshotbuilder.WithRequiredConsistency(true),
			)

			err := f.CreateWithDeferredDeletion(ctx, vdSnapshot)
			Expect(err).NotTo(HaveOccurred())
			waitVDSnapshotsReady(ctx, f, framework.LongTimeout, vdSnapshot)
		})

		By("Exporting VirtualDisk to local file", func() {
			exportData(ctx, f, "vd", vdData.Name, diskExportPath)
		})

		By("Exporting VirtualDiskSnapshot to local file", func() {
			exportData(ctx, f, "vds", vdSnapshot.Name, snapshotExportPath)
		})

		By("Creating disk from exported VirtualDisk", func() {
			vdFromDiskExport = createUploadDisk(ctx, f, "vd-restored-from-disk")
		})

		By("Uploading exported disk image", func() {
			uploadFile(ctx, f, vdFromDiskExport, diskExportPath)
		})

		By("Waiting for disk from VirtualDisk export to be ready", func() {
			waitDiskInExpectedPhase(ctx, f, vdFromDiskExport)
		})

		By("Creating disk from exported VirtualDiskSnapshot", func() {
			vdFromSnapshotExport = createUploadDisk(ctx, f, "vd-restored-from-snapshot")
		})

		By("Uploading exported snapshot image", func() {
			uploadFile(ctx, f, vdFromSnapshotExport, snapshotExportPath)
		})

		By("Waiting for disk from snapshot export to be ready", func() {
			waitDiskInExpectedPhase(ctx, f, vdFromSnapshotExport)
		})

		By("Attaching restored disks to VM", func() {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), vm)
			Expect(err).NotTo(HaveOccurred())

			vm.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: vdRoot.Name},
				{Kind: v1alpha2.DiskDevice, Name: vdFromDiskExport.Name},
				{Kind: v1alpha2.DiskDevice, Name: vdFromSnapshotExport.Name},
			}

			err = f.Clients.GenericClient().Update(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Starting the VM", func() {
			util.StartVirtualMachine(ctx, f, vm)
			Expect(vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)).To(Succeed())
		})

		By("Verifying data on disk restored from VirtualDisk export", func() {
			guestMount(ctx, f, vm, v1alpha2.DiskDevice, vdFromDiskExport.Name, mountPointData)
			restoredValue := guestReadFile(f, vm, fileDataPath)
			Expect(restoredValue).To(Equal(testFileValue), "Data should match original")
			guestUnmount(f, vm, mountPointData)
		})

		By("Verifying data on disk restored from VirtualDiskSnapshot export", func() {
			guestMount(ctx, f, vm, v1alpha2.DiskDevice, vdFromSnapshotExport.Name, mountPointData)
			restoredValue := guestReadFile(f, vm, fileDataPath)
			Expect(restoredValue).To(Equal(testFileValue), "Data should match original")
			guestUnmount(f, vm, mountPointData)
		})
	})
})

// waitDiskInExpectedPhase waits, via a VirtualDisk Observer, until vd reaches the
// phase expected for the default storage class' volume binding mode (Ready for
// Immediate, WaitForFirstConsumer otherwise).
func waitDiskInExpectedPhase(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk) {
	GinkgoHelper()
	expected := util.GetExpectedDiskPhaseByVolumeBindingMode()
	obs := vdobs.StartObserver(ctx, f, vd)
	obs.Never(vdobs.BeFailed())
	Expect(obs.WaitFor(func(d *v1alpha2.VirtualDisk) (bool, error) {
		return string(d.Status.Phase) == expected, nil
	}, framework.LongTimeout)).To(Succeed())
}

func IsNFS() bool {
	sc := framework.GetConfig().StorageClass.DefaultStorageClass
	if sc == nil {
		return false
	}
	return sc.Provisioner == framework.NFS
}

// runningOnClusterNode reports whether the test process runs on a Kubernetes
// node of the target cluster (its hostname matches a Node object). Off-cluster
// (e.g. a laptop connected over a kube-apiserver tunnel) the data-export
// download cannot use the in-cluster path and must fall back to publish mode.
func runningOnClusterNode(ctx context.Context, f *framework.Framework) bool {
	hostname, err := os.Hostname()
	Expect(err).NotTo(HaveOccurred(), "Failed to get hostname")
	var node corev1.Node
	err = f.Clients.GenericClient().Get(
		ctx,
		types.NamespacedName{Name: hostname},
		&node,
	)
	if k8serrors.IsNotFound(err) {
		return false
	}
	Expect(err).NotTo(HaveOccurred(), "Failed to get node %s", hostname)
	return true
}

// needPublishOption reports whether `d8 data export download` must be told to
// publish the exporter (true when the suite runs off-cluster).
func needPublishOption(ctx context.Context, f *framework.Framework) bool {
	return !runningOnClusterNode(ctx, f)
}

func exportData(ctx context.Context, f *framework.Framework, resourceType, name, outputFile string) {
	opts := d8.DataExportOptions{
		Namespace:  f.Namespace().Name,
		OutputFile: outputFile,
		Publish:    needPublishOption(ctx, f),
		Timeout:    framework.LongTimeout,
		Cleanup:    true,
	}
	if IsNFS() {
		opts.SourcePath = diskImageExportFile
	}
	err := f.D8Virtualization().DataExportDownload(resourceType, name, opts)
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		err := os.Remove(outputFile)
		Expect(err == nil || errors.Is(err, os.ErrNotExist)).To(BeTrue(), "Failed to remove exported file %s: %v", outputFile, err)
	})
}

func createUploadDisk(ctx context.Context, f *framework.Framework, name string) *v1alpha2.VirtualDisk {
	vd := vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(f.Namespace().Name),
		// Pin the same StorageClass the test reasons about: without it the disk
		// falls back to the module default (the cluster-default SC), which may
		// differ in VolumeBindingMode from config.DefaultStorageClass. On an
		// Immediate STORAGE_CLASS_NAME override the disk would otherwise land on
		// the WaitForFirstConsumer cluster default and never reach the Ready
		// phase that waitDiskInExpectedPhase expects.
		vdbuilder.WithStorageClass(defaultStorageClass()),
		vdbuilder.WithDatasource(&v1alpha2.VirtualDiskDataSource{
			Type: v1alpha2.DataSourceTypeUpload,
		}),
	)

	err := f.CreateWithDeferredDeletion(ctx, vd)
	Expect(err).NotTo(HaveOccurred())

	obs := vdobs.StartObserver(ctx, f, vd)
	obs.Never(vdobs.BeFailed())
	Expect(obs.WaitFor(vdobs.BeReadyForUserUpload(), framework.LongTimeout)).To(Succeed())

	return vd
}

func uploadFile(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk, filePath string) {
	err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vd), vd)
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

	// EXCEPTION: this retries an external HTTP upload endpoint, not a Kubernetes
	// resource or the guest, so Eventually is used deliberately. The uploader
	// Ingress may still return 503 from nginx for a few seconds after
	// ImageUploadURLs is published (IsUploaderReady probes via the Service
	// ClusterIP), so retry the upload until it stops returning 503.
	Eventually(func() error {
		err := doUploadAttempt(httpClient, uploadURL, filePath)
		if err != nil && !errors.Is(err, errUploadServiceUnavailable) {
			return StopTrying("upload failed with a non-retryable error").Wrap(err)
		}
		return err
	}, framework.ShortTimeout, 5*time.Second).Should(Succeed(), "Upload failed")
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

// errUploadServiceUnavailable marks a 503 response: the ingress controller has
// not started serving the upload Ingress yet, so the attempt can be retried.
var errUploadServiceUnavailable = errors.New("upload endpoint is not ready yet")

func handleUploadResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	if resp.StatusCode == http.StatusServiceUnavailable {
		return fmt.Errorf("%w: upload failed with status %d: %s", errUploadServiceUnavailable, resp.StatusCode, body)
	}

	return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, body)
}
