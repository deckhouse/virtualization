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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	// TODO(csi): re-add when the "VirtualDiskSnapshot" spec is re-enabled (see below).
	// vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const vdCreationBlankSize = "64Mi"

// progressUpdateInterval is the maximum time the reported import progress is
// allowed to stay unchanged between observed status updates. The controllers
// refresh progress roughly every two seconds while an importer is active, but
// the e2e contract allows a wider 10 second window.
//
// progressBoundaryBudget is the more lenient budget granted only to 0%, 50% and
// 100%, where provisioning may legitimately pause.
const (
	progressUpdateInterval = 10 * time.Second
	progressBoundaryBudget = time.Minute
)

var _ = Describe("VirtualDiskCreation", Label(
	precheck.PrecheckDefaultStorageClass,
), func() {
	var (
		f   *framework.Framework
		ctx context.Context

		scPtr *string
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "vd-creation")

		scPtr = defaultStorageClass()
	})

	It("provisions a VirtualDisk from HTTP data source", func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-http"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{URL: object.ImageURLAlpineBIOS}),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndRunVM(ctx, f, vd)
	})

	It("provisions a VirtualDisk from Upload data source", func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-upload"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDatasource(&v1alpha2.VirtualDiskDataSource{
				Type: v1alpha2.DataSourceTypeUpload,
			}),
			vdbuilder.WithStorageClass(scPtr),
		)

		var uploadFilePath string
		By("Downloading source image to upload", func() {
			var err error
			uploadFilePath, err = downloadImageToTempFile(object.ImageURLAlpineBIOS)
			Expect(err).NotTo(HaveOccurred(), "failed to download upload source image")
			DeferCleanup(func() {
				removeErr := os.Remove(uploadFilePath)
				Expect(removeErr == nil || errors.Is(removeErr, os.ErrNotExist)).To(BeTrue(),
					"failed to remove upload source file %q: %v", uploadFilePath, removeErr)
			})
		})

		obs := vdobs.StartObserver(ctx, f, vd)
		obs.Never(vdobs.BeFailed())
		obs.Always(vdobs.BeStorageClassReady())
		obs.Always(vdobs.BeDataSourceReady())
		obs.Always(vdobs.HaveValidPhaseTransitions())
		obs.Always(vdobs.HaveValidProgress(streamedVirtualDiskProgress(), progressUpdateInterval, progressBoundaryBudget))

		By("Creating VirtualDisk", func() {
			err := f.CreateWithDeferredDeletion(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for the VirtualDisk to expose upload URLs", func() {
			err := obs.WaitFor(vdobs.BeReadyForUserUpload(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Allowing ingress-nginx and the controller to reach the uploader pod (workaround)", func() {
			err := allowIngressToUploaderNetworkPolicy(ctx, f, vd.Namespace, vd.UID)
			Expect(err).NotTo(HaveOccurred(), "failed to patch uploader NetworkPolicy")
		})

		By("Uploading data to the VirtualDisk", func() {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vd), vd)
			Expect(err).NotTo(HaveOccurred())
			Expect(vd.Status.ImageUploadURLs).NotTo(BeNil())
			Expect(vd.Status.ImageUploadURLs.External).NotTo(BeEmpty())

			err = doRetriableUploadAttempt(vd.Status.ImageUploadURLs.External, uploadFilePath)
			Expect(err).NotTo(HaveOccurred(), "upload should succeed")
		})

		// On a WaitForFirstConsumer storage class the uploaded data lands in DVCR, and the
		// final import into the disk's volume only runs once the disk has a consumer; the
		// VirtualMachine created below is that consumer.
		runVirtualMachineFromDisks(ctx, f, observedDisk{vd: vd, obs: obs})
	})

	It("provisions a VirtualDisk from ContainerImage (registry) data source", func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-registry"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceContainerImage(object.ImageURLContainerImage, "", nil),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndRunVM(ctx, f, vd)
	})

	It("provisions a VirtualDisk from a VirtualImage on DVCR", func() {
		baseVI := vibuilder.New(
			vibuilder.WithName("vi-source-dvcr"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
			// The source image type is incidental here (the scenario tests a VD from a
			// VI on DVCR), so create the base image from a precreated ClusterVirtualImage.
			vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
		)

		viObs := viobs.StartObserver(ctx, f, baseVI)
		viObs.Never(viobs.BeFailed())
		viObs.Always(viobs.HaveFormat(expectedVirtualImageFormat(ctx, f, baseVI)))

		By("Creating base VirtualImage on DVCR", func() {
			err := f.CreateWithDeferredDeletion(ctx, baseVI)
			Expect(err).NotTo(HaveOccurred())

			err = viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		vd := vdbuilder.New(
			vdbuilder.WithName("vd-from-vi"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualImage, baseVI.Name),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndRunVM(ctx, f, vd, withIntermediateProgress())
	})

	It("provisions a VirtualDisk from a VirtualImage on PVC", func() {
		baseVI := vibuilder.New(
			vibuilder.WithName("vi-source-pvc"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceHTTP(object.ImageURLAlpineBIOS, nil, nil),
		)
		baseVI.Spec.PersistentVolumeClaim.StorageClass = scPtr

		viObs := viobs.StartObserver(ctx, f, baseVI)
		viObs.Never(viobs.BeFailed())
		viObs.Always(viobs.HaveFormat(expectedVirtualImageFormat(ctx, f, baseVI)))

		By("Creating base VirtualImage on PVC", func() {
			err := f.CreateWithDeferredDeletion(ctx, baseVI)
			Expect(err).NotTo(HaveOccurred())

			err = viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		vd := vdbuilder.New(
			vdbuilder.WithName("vd-from-vi-pvc"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualImage, baseVI.Name),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndRunVM(ctx, f, vd, withoutStreamingProgress())
	})

	// TODO(sc): disabled while VirtualDiskCreation is constrained to a single
	// default StorageClass. Re-enable when different-StorageClass scenarios are
	// needed again.
	/*
		It("provisions a VirtualDisk from a VirtualImage on PVC backed by a different storage class of the same CSI driver", func() {
			baseVI := vibuilder.New(
				vibuilder.WithName("vi-source-pvc-other-sc"),
				vibuilder.WithNamespace(f.Namespace().Name),
				vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
				// The source image type is incidental here (the scenario tests cloning from
				// a PVC-backed VI), so source the base image from a CVI.
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			)
			baseVI.Spec.PersistentVolumeClaim.StorageClass = scPtr

			viObs := viobs.StartObserver(ctx, f, baseVI)
			viObs.Never(viobs.BeFailed())
			viObs.Always(viobs.HaveFormat(expectedVirtualImageFormat(ctx, f, baseVI)))

			By("Creating base VirtualImage on PVC with the default storage class "+*scPtr, func() {
				err := f.CreateWithDeferredDeletion(ctx, baseVI)
				Expect(err).NotTo(HaveOccurred())

				err = viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			vd := vdbuilder.New(
				vdbuilder.WithName("vd-from-vi-other-sc"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualImage, baseVI.Name),
				vdbuilder.WithStorageClass(scPtr),
			)

			bootVD := vdbuilder.New(
				vdbuilder.WithName("vd-from-vi-other-sc-boot"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				// The boot disk is incidental here; the scenario checks that the
				// cloned disk provisions and attaches successfully.
				vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
				vdbuilder.WithStorageClass(scPtr),
			)

			bootObs := startVirtualDisk(ctx, f, bootVD, withIntermediateProgress())
			// PVC-backed source provisioning does not stream importer progress.
			cloneObs := startVirtualDisk(ctx, f, vd, withoutStreamingProgress())

			runVirtualMachineFromDisks(ctx, f,
				observedDisk{vd: bootVD, obs: bootObs},
				observedDisk{vd: vd, obs: cloneObs},
			)
		})
	*/

	It("provisions a VirtualDisk from a ClusterVirtualImage", func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-from-cvi"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndRunVM(ctx, f, vd, withIntermediateProgress())
	})

	It("provisions a blank VirtualDisk and attaches it to a running VirtualMachine", func() {
		blankVD := vdbuilder.New(
			vdbuilder.WithName("vd-blank"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithPersistentVolumeClaim(scPtr, ptr.To(resource.MustParse(vdCreationBlankSize))),
		)

		// A blank disk has no operating system, so the VM boots from a bootable
		// VirtualDisk and the blank disk is attached as an additional volume. Both disks
		// are created first and the VM provides the consumer that triggers provisioning
		// (required for WaitForFirstConsumer storage classes).
		bootVD := vdbuilder.New(
			vdbuilder.WithName("vd-blank-boot"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			// The boot disk is incidental here (the scenario tests the blank disk), so
			// source it from a precreated ClusterVirtualImage instead of HTTP.
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			vdbuilder.WithStorageClass(scPtr),
		)

		bootObs := startVirtualDisk(ctx, f, bootVD, withIntermediateProgress())
		// A blank disk is provisioned by the CSI driver and may legitimately jump
		// straight from 0% to 100%.
		blankObs := startVirtualDisk(ctx, f, blankVD, withoutStreamingProgress())

		runVirtualMachineFromDisks(ctx, f,
			observedDisk{vd: bootVD, obs: bootObs},
			observedDisk{vd: blankVD, obs: blankObs},
		)
	})

	// TODO(csi): temporarily disabled for the same reason as "VirtualImage on PVC" above.
	// A VirtualDisk created from a VirtualDiskSnapshot uses the CSI snapshot-clone/restore
	// path, which leaves the volume DRBD-Primary on a node other than the VirtualMachine's,
	// so DRBD (single-primary) refuses to promote it read-write on the VM's node
	// ("failed to set source device readwrite"). Re-enable once snapshot-sourced disks are
	// materialized into a fresh volume owned solely by the VM's node.
	/*
		Context("with snapshots", Label(precheck.PrecheckSnapshot), func() {
			It("provisions a VirtualDisk from a VirtualDiskSnapshot", func() {
				baseVD := vdbuilder.New(
					vdbuilder.WithName("vd-source-for-snapshot"),
					vdbuilder.WithNamespace(f.Namespace().Name),
					vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{URL: object.ImageURLAlpineBIOS}),
					vdbuilder.WithStorageClass(scPtr),
				)

				// Boot a VM from the source disk so it provisions (the VM is its consumer on
				// WaitForFirstConsumer storage classes) and so the consistent snapshot below
				// can freeze the guest filesystem via the agent.
				createVirtualDiskAndRunVM(ctx, f, baseVD)

				vdSnapshot := vdsnapshotbuilder.New(
					vdsnapshotbuilder.WithName("vd-snapshot"),
					vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
					vdsnapshotbuilder.WithVirtualDiskName(baseVD.Name),
					vdsnapshotbuilder.WithRequiredConsistency(true),
				)

				By("Creating VirtualDiskSnapshot", func() {
					err := f.CreateWithDeferredDeletion(ctx, vdSnapshot)
					Expect(err).NotTo(HaveOccurred())

					util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.LongTimeout, vdSnapshot)
				})

				vd := vdbuilder.New(
					vdbuilder.WithName("vd-from-snapshot"),
					vdbuilder.WithNamespace(f.Namespace().Name),
					vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
					vdbuilder.WithStorageClass(scPtr),
				)

				createVirtualDiskAndRunVM(ctx, f, vd)
			})
		})
	*/
})

// wffcStorageClass returns a pointer to the name of the WaitForFirstConsumer StorageClass
// that block-device tests use to provision the scenario's main VirtualDisks and
// VirtualImages. Its presence and WaitForFirstConsumer volume binding mode are enforced by
// precheck.PrecheckWFFCStorageClass.
func wffcStorageClass() *string {
	GinkgoHelper()

	sc := framework.GetConfig().StorageClass.WFFCStorageClass
	Expect(sc).NotTo(BeNil(),
		"WFFC StorageClass not found: set %s or configure a default StorageClass (enforced by the %q precheck)",
		config.WFFCStorageClassEnv, precheck.PrecheckWFFCStorageClass)

	return ptr.To(sc.Name)
}

// immediateStorageClass returns a pointer to the name of the immediate StorageClass, used as
// the "other" StorageClass (same CSI driver as the WFFC one) when a source object must
// live on a different StorageClass than the produced one, and to provision dependent objects
// that must become Ready without a consumer. Its presence and Immediate volume binding mode are
// enforced by precheck.PrecheckImmediateStorageClass; the shared CSI driver with the WFFC
// StorageClass is enforced by precheck.PrecheckSameCSIDriverStorageClass.
func immediateStorageClass() *string {
	GinkgoHelper()

	sc := framework.GetConfig().StorageClass.ImmediateStorageClass
	Expect(sc).NotTo(BeNil(),
		"immediate StorageClass not found: set %s or configure a default StorageClass (enforced by the %q precheck)",
		config.ImmediateStorageClassEnv, precheck.PrecheckImmediateStorageClass)

	return ptr.To(sc.Name)
}

// expectedDiskPhaseBeforeVM returns the phase predicate the disk must satisfy before its
// consuming VirtualMachine is created: a WaitForFirstConsumer disk parks in the
// WaitForFirstConsumer phase until the VM (its consumer) is scheduled, while an Immediate
// disk provisions to Ready on its own.
func expectedDiskPhaseBeforeVM(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk) vdobs.Predicate {
	GinkgoHelper()

	if storageClassIsWaitForFirstConsumer(ctx, f, ptr.Deref(vd.Spec.PersistentVolumeClaim.StorageClass, "")) {
		return vdobs.BeWaitForFirstConsumer()
	}
	return vdobs.BeReady()
}

// storageClassIsWaitForFirstConsumer reports whether the named StorageClass (or the cluster
// default, when name is empty) uses the WaitForFirstConsumer volume binding mode.
func storageClassIsWaitForFirstConsumer(ctx context.Context, f *framework.Framework, name string) bool {
	GinkgoHelper()

	var sc *storagev1.StorageClass
	if name == "" {
		sc = framework.GetConfig().StorageClass.DefaultStorageClass
		Expect(sc).NotTo(BeNil(), "default StorageClass not found")
	} else {
		got, err := f.KubeClient().StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "failed to get StorageClass %q", name)
		sc = got
	}

	return sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
}

// setupProject creates a non-isolated Deckhouse Project, waits until it is deployed and
// switches the framework to operate inside the project's namespace. The project (and
// therefore its namespace and every resource it contains) is removed during cleanup.
//
// The project uses the "NotRestricted" network policy: these tests boot VirtualMachines
// whose guests need outbound network access (cloud-init installs the qemu-guest-agent over
// the network), which the "Isolated" policy would block, leaving the guest agent forever
// not ready. Network-isolation behaviour is covered separately by the ImporterNetworkPolicy
// spec.
func setupProject(ctx context.Context, f *framework.Framework, prefix string) {
	GinkgoHelper()

	project := object.NewNonIsolatedProject(prefix, framework.NamespaceBasePrefix)

	By("Creating a Project", func() {
		err := f.CreateWithDeferredDeletion(ctx, project)
		Expect(err).NotTo(HaveOccurred())

		util.UntilObjectState(ctx, "Deployed", framework.ShortTimeout, project)
	})

	f.SetProjectNamespace(project.Name)
}

// startVirtualDisk starts the VirtualDisk observer with the standard invariants and
// creates vd, WITHOUT waiting for it to become Ready. The returned observer keeps
// enforcing the invariants until cleanup. Waiting for readiness is left to the caller
// because, on WaitForFirstConsumer storage classes, a VirtualDisk only provisions once a
// consumer (a VirtualMachine) is scheduled.
func startVirtualDisk(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk, opts ...progressWaitOption) vdobs.Observer {
	GinkgoHelper()

	var o progressWaitOptions
	for _, fn := range opts {
		fn(&o)
	}

	obs := vdobs.StartObserver(ctx, f, vd)
	obs.Never(vdobs.BeFailed())
	obs.Always(vdobs.BeStorageClassReady())
	obs.Always(vdobs.BeDataSourceReady())
	obs.Always(vdobs.HaveValidPhaseTransitions())
	obs.Always(vdobs.HaveValidProgress(virtualDiskProgressExpectations(vd, o), progressUpdateInterval, progressBoundaryBudget))

	By("Creating VirtualDisk", func() {
		err := f.CreateWithDeferredDeletion(ctx, vd)
		Expect(err).NotTo(HaveOccurred())
	})

	return obs
}

func virtualDiskProgressExpectations(vd *v1alpha2.VirtualDisk, o progressWaitOptions) vdobs.ProgressExpectations {
	if o.skipStreamingProgress || o.skipDiskStreamingProgress {
		return vdobs.ProgressExpectations{
			RequireZero:    true,
			RequireHundred: true,
		}
	}
	if isVirtualDiskFromCVI(vd) {
		return vdobs.ProgressExpectations{
			RequireZero:    true,
			RequireHundred: true,
		}
	}
	if o.progressCoverage == progressCoverageIntermediate {
		return vdobs.ProgressExpectations{
			RequireZero:                    true,
			RequireIntermediateExceptFifty: true,
			RequireHundred:                 true,
		}
	}
	return streamedVirtualDiskProgress()
}

func isVirtualDiskFromCVI(vd *v1alpha2.VirtualDisk) bool {
	return vd.Spec.DataSource != nil &&
		vd.Spec.DataSource.ObjectRef != nil &&
		vd.Spec.DataSource.ObjectRef.Kind == v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage
}

func streamedVirtualDiskProgress() vdobs.ProgressExpectations {
	return vdobs.ProgressExpectations{
		RequireZero:                    true,
		RequireBetweenZeroAndFifty:     true,
		RequireBetweenFiftyAndHundred:  true,
		RequireIntermediateExceptFifty: true,
		RequireHundred:                 true,
	}
}

// createVirtualDiskAndWait provisions vd and waits until it becomes Ready. It must only
// be used for disks that provision without a VirtualMachine (e.g. an Upload disk, whose
// uploader Pod is the consumer); on WaitForFirstConsumer storage classes a disk without
// a consumer never becomes Ready - use createVirtualDiskAndRunVM instead.
func createVirtualDiskAndWait(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk) {
	GinkgoHelper()

	obs := startVirtualDisk(ctx, f, vd)
	err := obs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
}

// observedDisk pairs a VirtualDisk with the observer watching its lifecycle, as
// returned by startVirtualDisk.
type observedDisk struct {
	vd  *v1alpha2.VirtualDisk
	obs vdobs.Observer
}

// virtualDiskNoun returns the singular or plural "VirtualDisk(s)" noun for use
// in step messages, depending on how many disks are involved.
func virtualDiskNoun(n int) string {
	if n == 1 {
		return "VirtualDisk"
	}
	return "VirtualDisks"
}

// createVirtualDiskAndRunVM provisions vd by booting a VirtualMachine from it (see
// runVirtualMachineFromDisks for the exact lifecycle). The VM is the disk's first
// consumer, so this works on both Immediate and WaitForFirstConsumer storage classes.
func createVirtualDiskAndRunVM(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk, opts ...progressWaitOption) {
	GinkgoHelper()

	obs := startVirtualDisk(ctx, f, vd, opts...)
	runVirtualMachineFromDisks(ctx, f, observedDisk{vd: vd, obs: obs})
}

// runVirtualMachineFromDisks drives the disk/VM lifecycle for the given disks, which the
// caller has already created via startVirtualDisk (the first disk is the boot disk):
//
//  1. (the disks are already created by the caller);
//  2. wait every disk to be Ready or waiting for a consumer (WaitForFirstConsumer);
//  3. create the VirtualMachine that consumes the disks;
//  4. wait every disk to become Ready (a WaitForFirstConsumer disk provisions only once
//     the VirtualMachine is scheduled as its consumer);
//  5. wait the VirtualMachine to be Running;
//  6. wait the VirtualMachine guest agent to be ready.
func runVirtualMachineFromDisks(ctx context.Context, f *framework.Framework, disks ...observedDisk) {
	GinkgoHelper()

	noun := virtualDiskNoun(len(disks))

	By(fmt.Sprintf("Waiting for the %s to settle before creating the VirtualMachine", noun), func() {
		for _, d := range disks {
			// A WaitForFirstConsumer disk must park in WaitForFirstConsumer (it provisions
			// only once the VM consumer is scheduled); an Immediate disk must become Ready.
			err := d.obs.WaitFor(expectedDiskPhaseBeforeVM(ctx, f, d.vd), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	vds := make([]*v1alpha2.VirtualDisk, len(disks))
	for i := range disks {
		vds[i] = disks[i].vd
	}
	vm := object.NewMinimalVM("vm-from-disk-", f.Namespace().Name,
		vmbuilder.WithDisks(vds...),
	)

	By(fmt.Sprintf("Creating VirtualMachine from the %s", noun), func() {
		err := f.CreateWithDeferredDeletion(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
	})

	// Start the observer only after Create: the VM uses a generated name, so its
	// name is unknown (and the observer cannot match its watch events) until the
	// API server assigns it during Create.
	vmObs := vmobs.StartObserver(ctx, f, vm)
	vmObs.Never(vmobs.BeFailed())
	// Fail fast instead of blocking on the guest-agent wait: a VM that reports
	// NoBootableDevice will never boot an OS and never bring up its agent.
	vmObs.Never(vmobs.HaveNoBootableDevice())

	By(fmt.Sprintf("Waiting for the %s to be Ready", noun), func() {
		for _, d := range disks {
			err := d.obs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())

			err = f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(d.vd), d.vd)
			Expect(err).NotTo(HaveOccurred())
			expectVirtualDiskBlockStorage(ctx, f, d.vd)
		}
	})

	By("Waiting for the VirtualMachine to be Running", func() {
		err := vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Waiting for the guest agent to be ready", func() {
		err := vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})
}

func doRetriableUploadAttempt(url, filePath string) error {
	const maxAttempts = 12
	const retryDelay = 5 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := doVirtualDiskUploadAttempt(url, filePath)
		if err == nil {
			return nil
		}
		if !isRetriableUploadError(err) {
			return err
		}

		lastErr = err
		time.Sleep(retryDelay)
	}

	return fmt.Errorf("upload failed after %d attempts: %w", maxAttempts, lastErr)
}

func doVirtualDiskUploadAttempt(url, filePath string) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

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

func isRetriableUploadError(err error) bool {
	message := err.Error()
	return !strings.Contains(message, "upload failed with status ") ||
		strings.Contains(message, "upload failed with status 5")
}

// downloadImageToTempFile downloads url into a temporary file and returns its path.
// The caller is responsible for removing the file when finished.
func downloadImageToTempFile(url string) (string, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %q: %w", url, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "failed to close response body")
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download %q: unexpected status %d", url, resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", filepath.Base(url)+"-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	closed := false
	defer func() {
		if closed {
			return
		}
		if closeErr := tmpFile.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "failed to close temp file")
		}
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", fmt.Errorf("copy to temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	closed = true

	return tmpFile.Name(), nil
}

// uploaderIngressNginxNamespaceLabel is the namespace label used to match the
// Deckhouse ingress-nginx controller namespace.
const uploaderIngressNginxNamespaceLabel = "module"

// uploaderIngressNginxNamespaceLabelValue is the value of the namespace label
// for the Deckhouse ingress-nginx controller namespace (d8-ingress-nginx).
const uploaderIngressNginxNamespaceLabelValue = "ingress-nginx"

// controllerNamespaceLabel / controllerNamespaceLabelValue match the namespace
// where the virtualization-controller runs (d8-virtualization).
const (
	controllerNamespaceLabel      = "kubernetes.io/metadata.name"
	controllerNamespaceLabelValue = "d8-virtualization"
)

// allowIngressToUploaderNetworkPolicy patches the NetworkPolicy created by the
// virtualization-controller for the uploader pod owned by vd, so that traffic
// from the namespaces the upload flow depends on is allowed to reach the
// uploader pod:
//
//   - d8-ingress-nginx (label "module=ingress-nginx"): without it external
//     uploads via the Ingress URL fail with a 504 Gateway Time-out.
//   - d8-virtualization (the virtualization-controller namespace): the
//     controller scrapes the uploader's progress metrics over the pod IP. As
//     soon as any ingress rule is present on the pod, Cilium starts enforcing
//     ingress and would otherwise drop the controller's scrape, which makes the
//     reported upload progress stay stuck at 0% and jump straight to 50% only
//     when the uploader pod completes. Allowing d8-virtualization keeps the
//     live progress flowing (0% -> 50%).
func allowIngressToUploaderNetworkPolicy(ctx context.Context, f *framework.Framework, namespace string, ownerUID types.UID) error {
	var policies netv1.NetworkPolicyList
	if err := f.Clients.GenericClient().List(ctx, &policies, crclient.InNamespace(namespace)); err != nil {
		return fmt.Errorf("list network policies in %q: %w", namespace, err)
	}

	requiredPeers := []map[string]string{
		{uploaderIngressNginxNamespaceLabel: uploaderIngressNginxNamespaceLabelValue},
		{controllerNamespaceLabel: controllerNamespaceLabelValue},
	}

	var patched int
	for i := range policies.Items {
		np := &policies.Items[i]
		if !isOwnedByUID(np.OwnerReferences, ownerUID) {
			continue
		}

		var changed bool
		for _, labels := range requiredPeers {
			if hasNamespaceSelectorPeer(np.Spec.Ingress, labels) {
				continue
			}
			if len(np.Spec.Ingress) == 0 {
				np.Spec.Ingress = []netv1.NetworkPolicyIngressRule{{}}
			}
			np.Spec.Ingress[0].From = append(np.Spec.Ingress[0].From, netv1.NetworkPolicyPeer{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: labels},
			})
			changed = true
		}

		if changed {
			if err := f.Clients.GenericClient().Update(ctx, np); err != nil {
				return fmt.Errorf("update network policy %q: %w", np.Name, err)
			}
		}
		patched++
	}

	if patched == 0 {
		return fmt.Errorf("no NetworkPolicy owned by UID %q found in %q", ownerUID, namespace)
	}
	return nil
}

// expectVirtualDiskBlockStorage verifies that a Ready VirtualDisk target PVC is a
// block volume. Block-device tests store flat raw disks on such volumes.
func expectVirtualDiskBlockStorage(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk) {
	GinkgoHelper()

	Expect(vd.Status.Target.PersistentVolumeClaim).NotTo(BeEmpty())

	pvc := &corev1.PersistentVolumeClaim{}
	err := f.Clients.GenericClient().Get(ctx, types.NamespacedName{
		Name:      vd.Status.Target.PersistentVolumeClaim,
		Namespace: vd.Namespace,
	}, pvc)
	Expect(err).NotTo(HaveOccurred(), "failed to get target PVC for VirtualDisk %q", vd.Name)
	Expect(pvc.Spec.VolumeMode).NotTo(BeNil())
	Expect(*pvc.Spec.VolumeMode).To(Equal(corev1.PersistentVolumeBlock))
}

func isOwnedByUID(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

func hasNamespaceSelectorPeer(rules []netv1.NetworkPolicyIngressRule, labels map[string]string) bool {
	for _, rule := range rules {
		for _, from := range rule.From {
			if from.NamespaceSelector == nil {
				continue
			}
			if equalLabels(from.NamespaceSelector.MatchLabels, labels) {
				return true
			}
		}
	}
	return false
}

func equalLabels(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
