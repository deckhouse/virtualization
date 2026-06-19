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
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdsnapshotobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vdsnapshot"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("VirtualImageCreation", Label(
	precheck.PrecheckWFFCStorageClass,
	precheck.PrecheckImmediateStorageClass,
	precheck.PrecheckSameCSIDriverStorageClass,
), func() {
	var (
		f   *framework.Framework
		ctx context.Context

		scPtr          *string
		immediateSCPtr *string
	)

	// setup provisions a fresh framework, a dedicated Project and the storage class
	// pointers. It is invoked from a BeforeEach for independent specs (each spec gets
	// its own Project, so the DVCR and PVC specs can run in parallel) and from a
	// BeforeAll for specs that share a common dependency created once for the whole
	// Ordered container.
	setup := func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "vi-creation")

		scPtr = wffcStorageClass()
		immediateSCPtr = immediateStorageClass()
	}

	// The DVCR and PVC specs below do not share an in-cluster dependency, so each gets
	// its own Project via BeforeEach and the two specs can run in parallel.

	Context("from HTTP data source", func() {
		BeforeEach(setup)

		It("provisions a VirtualImage on DVCR", func() {
			vi := newVirtualImageOnDVCR("vi-http",
				vibuilder.WithDataSourceHTTP(object.ImageURLAlpineBIOS, nil, nil),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})

		It("provisions a VirtualImage on PVC", func() {
			vi := newVirtualImageOnPVC("vi-pvc-http", scPtr,
				vibuilder.WithDataSourceHTTP(object.ImageURLAlpineBIOS, nil, nil),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})
	})

	Context("from ContainerImage (registry) data source", func() {
		BeforeEach(setup)

		It("provisions a VirtualImage on DVCR", func() {
			vi := newVirtualImageOnDVCR("vi-registry",
				vibuilder.WithDataSourceContainerImage(object.ImageURLContainerImage, v1alpha2.ImagePullSecretName{}, nil),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})

		It("provisions a VirtualImage on PVC", func() {
			vi := newVirtualImageOnPVC("vi-pvc-registry", scPtr,
				vibuilder.WithDataSourceContainerImage(object.ImageURLContainerImage, v1alpha2.ImagePullSecretName{}, nil),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})
	})

	Context("from a ClusterVirtualImage", func() {
		BeforeEach(setup)

		It("provisions a VirtualImage on DVCR", func() {
			vi := newVirtualImageOnDVCR("vi-from-cvi",
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})

		It("provisions a VirtualImage on PVC", func() {
			vi := newVirtualImageOnPVC("vi-pvc-from-cvi", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})
	})

	// The specs below share an in-cluster (or downloaded) dependency, so it is created
	// once in a BeforeAll and reused by both the DVCR and PVC specs of an Ordered
	// container. Different Ordered containers still run in parallel across processes.

	Context("from Upload data source", Ordered, func() {
		var uploadFilePath string

		BeforeAll(func() {
			setup()

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
		})

		It("provisions a VirtualImage on DVCR", func() {
			vi := newVirtualImageOnDVCR("vi-upload",
				vibuilder.WithDatasource(v1alpha2.VirtualImageDataSource{
					Type: v1alpha2.DataSourceTypeUpload,
				}),
			)
			uploadVirtualImageAndWait(ctx, f, vi, uploadFilePath)
			runVirtualMachineFromImageDisk(ctx, f, vi)
		})

		It("provisions a VirtualImage on PVC", func() {
			vi := newVirtualImageOnPVC("vi-pvc-upload", scPtr,
				vibuilder.WithDatasource(v1alpha2.VirtualImageDataSource{
					Type: v1alpha2.DataSourceTypeUpload,
				}),
			)
			uploadVirtualImageAndWait(ctx, f, vi, uploadFilePath)
			runVirtualMachineFromImageDisk(ctx, f, vi)
		})
	})

	Context("from a VirtualDisk", Ordered, func() {
		var vd *v1alpha2.VirtualDisk

		BeforeAll(func() {
			setup()
			// The source VirtualDisk is a dependency that must become Ready without a
			// VirtualMachine consumer, so it lives on the immediate storage class.
			vd = createSourceVirtualDiskAndWait(ctx, f, "vd-source-for-vi", immediateSCPtr)
		})

		It("provisions a VirtualImage on DVCR", func() {
			vi := newVirtualImageOnDVCR("vi-from-vd",
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})

		It("provisions a VirtualImage on PVC", func() {
			vi := newVirtualImageOnPVC("vi-pvc-from-vd", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			createVirtualImageAndRunVM(ctx, f, vi, withoutStreamingProgress())
		})
	})

	Context("from a VirtualImage on DVCR", Ordered, func() {
		var baseVI *v1alpha2.VirtualImage

		BeforeAll(func() {
			setup()
			baseVI = newVirtualImageOnDVCR("vi-source-dvcr",
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			)
			createVirtualImageAndWait(ctx, f, baseVI)
		})

		It("provisions a VirtualImage on DVCR", func() {
			vi := newVirtualImageOnDVCR("vi-from-vi-dvcr",
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})

		It("provisions a VirtualImage on PVC", func() {
			vi := newVirtualImageOnPVC("vi-pvc-from-vi-dvcr", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})
	})

	Context("from a VirtualImage on PVC", Ordered, func() {
		var baseVI *v1alpha2.VirtualImage

		BeforeAll(func() {
			setup()
			// The source VirtualImage is a dependency on PVC, created on the immediate
			// storage class so it becomes Ready on its own.
			baseVI = newVirtualImageOnPVC("vi-source-pvc", immediateSCPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			)
			createVirtualImageAndWait(ctx, f, baseVI)
		})

		It("provisions a VirtualImage on DVCR", func() {
			vi := newVirtualImageOnDVCR("vi-from-vi-pvc",
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})

		It("provisions a VirtualImage on PVC", func() {
			vi := newVirtualImageOnPVC("vi-pvc-from-vi-pvc", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
			)
			// PVC-to-PVC on the same CSI driver provisions via a CSI clone, which is
			// instantaneous and does not stream intermediate progress.
			createVirtualImageAndRunVM(ctx, f, vi, withoutStreamingProgress())
		})
	})

	// PVC-only scenarios: the source resides on a different storage class of the same
	// CSI driver. Each spec creates its own source, so they are independent and use a
	// per-spec Project via BeforeEach.
	Context("on PVC from a source on a different storage class of the same CSI driver", func() {
		BeforeEach(setup)

		It("provisions a VirtualImage from a VirtualDisk", func() {
			vd := createSourceVirtualDiskAndWait(ctx, f, "vd-source-for-vi-other-sc", immediateSCPtr)

			vi := newVirtualImageOnPVC("vi-pvc-from-vd-other-sc", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDisk, vd.Name),
			)
			// Same-CSI PVC source provisions via a CSI clone (no streamed progress).
			createVirtualImageAndRunVM(ctx, f, vi, withoutStreamingProgress())
		})

		It("provisions a VirtualImage from a VirtualImage", func() {
			baseVI := newVirtualImageOnPVC("vi-source-pvc-other-sc", immediateSCPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
			)
			createVirtualImageAndWait(ctx, f, baseVI)

			vi := newVirtualImageOnPVC("vi-pvc-from-vi-other-sc", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualImage, baseVI.Name),
			)
			// Same-CSI PVC source provisions via a CSI clone (no streamed progress).
			createVirtualImageAndRunVM(ctx, f, vi, withoutStreamingProgress())
		})
	})

	Context("from a VirtualDiskSnapshot", Ordered, Label(precheck.PrecheckSnapshot), func() {
		var vdSnapshot *v1alpha2.VirtualDiskSnapshot

		BeforeAll(func() {
			setup()
			// The source VirtualDisk is a dependency that must become Ready without a
			// VirtualMachine consumer, so it lives on the immediate storage class.
			vd := createSourceVirtualDiskAndWait(ctx, f, "vd-source-for-vi-snapshot", immediateSCPtr)

			vdSnapshot = vdsnapshotbuilder.New(
				vdsnapshotbuilder.WithName("vdsnapshot-source-for-vi"),
				vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vdsnapshotbuilder.WithVirtualDiskName(vd.Name),
				vdsnapshotbuilder.WithRequiredConsistency(true),
			)

			snapObs := vdsnapshotobs.StartObserver(ctx, f, vdSnapshot)
			By("Creating VirtualDiskSnapshot", func() {
				err := f.CreateWithDeferredDeletion(ctx, vdSnapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(snapObs.WaitFor(vdsnapshotobs.BeReady(), framework.LongTimeout)).NotTo(HaveOccurred())
			})
		})

		It("provisions a VirtualImage on DVCR", func() {
			vi := newVirtualImageOnDVCR("vi-from-vdsnapshot",
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			createVirtualImageAndRunVM(ctx, f, vi)
		})

		It("provisions a VirtualImage on PVC", func() {
			vi := newVirtualImageOnPVC("vi-pvc-from-vdsnapshot", scPtr,
				vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
			)
			createVirtualImageAndRunVM(ctx, f, vi, withoutStreamingProgress())
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

// progressWaitOptions tunes the progress coverage expected while waiting for a
// VirtualImage or VirtualDisk to become Ready.
type progressWaitOptions struct {
	// skipStreamingProgress selects the minimal progress coverage (0% and
	// 100%). It is used by bind-based provisioning such as a CSI clone, which
	// does not report intermediate percentages.
	skipStreamingProgress bool
	// skipDiskStreamingProgress relaxes only the downstream VirtualDisk progress
	// check used by createVirtualImageAndRunVM. A PVC-backed VirtualImage can be
	// imported with streamed progress, while the disk created from it is then a
	// same-CSI PVC clone with no importer pod to report intermediate percentages.
	skipDiskStreamingProgress bool
}

type progressWaitOption func(*progressWaitOptions)

// withoutStreamingProgress selects minimal progress coverage for resources
// provisioned via a CSI clone (a VirtualImage/VirtualDisk on PVC created from a
// PVC-backed source on the same CSI driver), where there is no importer pod to
// report intermediate percentages.
func withoutStreamingProgress() progressWaitOption {
	return func(o *progressWaitOptions) { o.skipStreamingProgress = true }
}

func withoutDiskStreamingProgress() progressWaitOption {
	return func(o *progressWaitOptions) { o.skipDiskStreamingProgress = true }
}

func createVirtualImageAndWait(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage, opts ...progressWaitOption) {
	GinkgoHelper()

	var o progressWaitOptions
	for _, fn := range opts {
		fn(&o)
	}

	vi.Namespace = f.Namespace().Name
	obs := viobs.StartObserver(ctx, f, vi)
	obs.Never(viobs.BeFailed())
	obs.Always(viobs.HaveValidPhaseTransitions())
	obs.Always(viobs.HaveValidProgress(virtualImageProgressExpectations(vi, o), progressUpdateInterval, progressBoundaryBudget))

	By("Creating VirtualImage on "+virtualImageStorageName(vi), func() {
		err := f.CreateWithDeferredDeletion(ctx, vi)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Waiting for the VirtualImage to be Ready", func() {
		Expect(obs.WaitFor(viobs.BeReady(), framework.LongTimeout)).NotTo(HaveOccurred())
	})
}

func uploadVirtualImageAndWait(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage, uploadFilePath string) {
	GinkgoHelper()

	vi.Namespace = f.Namespace().Name
	obs := viobs.StartObserver(ctx, f, vi)
	obs.Never(viobs.BeFailed())
	obs.Always(viobs.HaveValidPhaseTransitions())
	obs.Always(viobs.HaveValidProgress(virtualImageProgressExpectations(vi, progressWaitOptions{}), progressUpdateInterval, progressBoundaryBudget))

	By("Creating VirtualImage on "+virtualImageStorageName(vi), func() {
		err := f.CreateWithDeferredDeletion(ctx, vi)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Waiting for the VirtualImage to expose upload URLs", func() {
		err := obs.WaitFor(viobs.BeReadyForUserUpload(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Allowing ingress-nginx and the controller to reach the uploader pod (workaround)", func() {
		err := allowIngressToUploaderNetworkPolicy(ctx, f, vi.Namespace, vi.UID)
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

	By("Waiting for the VirtualImage to be Ready", func() {
		Expect(obs.WaitFor(viobs.BeReady(), framework.LongTimeout)).NotTo(HaveOccurred())
	})
}

func virtualImageProgressExpectations(vi *v1alpha2.VirtualImage, o progressWaitOptions) viobs.ProgressExpectations {
	if vi.Spec.Storage == v1alpha2.StorageContainerRegistry {
		return viobs.ProgressExpectations{
			RequireZero:                    true,
			RequireIntermediateExceptFifty: true,
			RequireHundred:                 true,
		}
	}
	if o.skipStreamingProgress {
		return viobs.ProgressExpectations{
			RequireZero:    true,
			RequireHundred: true,
		}
	}
	return viobs.ProgressExpectations{
		RequireZero:                    true,
		RequireBetweenZeroAndFifty:     true,
		RequireBetweenFiftyAndHundred:  true,
		RequireIntermediateExceptFifty: true,
		RequireHundred:                 true,
	}
}

// createVirtualImageAndRunVM provisions a (qcow2) VirtualImage, provisions a VirtualDisk
// from it, and boots a VirtualMachine from that disk. A VirtualImage cannot occupy a VM's
// first block-device slot, so booting always goes through a VirtualDisk; the VM is run
// until it is Running and its guest agent is ready.
func createVirtualImageAndRunVM(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage, opts ...progressWaitOption) {
	GinkgoHelper()

	createVirtualImageAndWait(ctx, f, vi, opts...)
	runVirtualMachineFromImageDisk(ctx, f, vi, opts...)
}

// runVirtualMachineFromImageDisk provisions a VirtualDisk from the (Ready) VirtualImage
// and boots a VirtualMachine from that disk, waiting until the VM is Running and its guest
// agent is ready.
func runVirtualMachineFromImageDisk(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage, opts ...progressWaitOption) {
	GinkgoHelper()

	if vi.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim {
		opts = append(opts, withoutDiskStreamingProgress())
	}

	// The disk that boots the VM is the scenario's main resource, so it lives on the WFFC
	// storage class.
	vd := object.NewVDFromVI("vd-from-"+vi.Name, f.Namespace().Name, vi,
		vdbuilder.WithStorageClass(wffcStorageClass()),
	)
	createVirtualDiskAndRunVM(ctx, f, vd, opts...)
}

func createSourceVirtualDiskAndWait(ctx context.Context, f *framework.Framework, name string, sc *string) *v1alpha2.VirtualDisk {
	GinkgoHelper()

	vd := vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(f.Namespace().Name),
		// Incidental source disk: provision from a precreated ClusterVirtualImage.
		vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVIAlpineBIOS),
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
