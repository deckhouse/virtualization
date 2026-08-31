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

package source

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	servicestat "github.com/deckhouse/virtualization-controller/pkg/controller/service/stat"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type sourcesHandlerStub struct {
	cleanupRequeue bool
	cleanupReason  string
	cleanupErr     error
	cleanupCalls   int
}

func (s *sourcesHandlerStub) StoreToDVCR(context.Context, *v1alpha2.VirtualImage) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (s *sourcesHandlerStub) StoreToPVC(context.Context, *v1alpha2.VirtualImage) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (s *sourcesHandlerStub) CleanUp(context.Context, *v1alpha2.VirtualImage) (bool, string, error) {
	s.cleanupCalls++
	return s.cleanupRequeue, s.cleanupReason, s.cleanupErr
}

func (s *sourcesHandlerStub) Validate(context.Context, *v1alpha2.VirtualImage) error {
	return nil
}

type sourcesCleanerStub struct {
	cleanupRequeue           bool
	cleanupReason            string
	cleanupErr               error
	cleanupSupplementsResult reconcile.Result
	cleanupSupplementsErr    error
	cleanupCalls             int
	cleanupSupplementsCalls  int
}

func (s *sourcesCleanerStub) CleanUp(context.Context, *v1alpha2.VirtualImage) (bool, string, error) {
	s.cleanupCalls++
	return s.cleanupRequeue, s.cleanupReason, s.cleanupErr
}

func (s *sourcesCleanerStub) CleanUpSupplements(context.Context, *v1alpha2.VirtualImage) (reconcile.Result, error) {
	s.cleanupSupplementsCalls++
	return s.cleanupSupplementsResult, s.cleanupSupplementsErr
}

var _ = Describe("Sources helpers", func() {
	newVI := func() *v1alpha2.VirtualImage {
		return &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "vi",
				Namespace:   "default",
				UID:         "vi-uid",
				Annotations: map[string]string{},
			},
		}
	}

	newScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(netv1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	newBoundImportPVC := func(name, namespace string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				UID:         types.UID(name + "-uid"),
				Annotations: map[string]string{},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		}
	}

	Describe("Sources map operations", func() {
		It("stores handlers, resolves them and detects changes", func() {
			sources := NewSources()
			handler := &sourcesHandlerStub{}
			vi := newVI()
			vi.Generation = 2
			vi.Status.ObservedGeneration = 1

			sources.Set(v1alpha2.DataSourceTypeObjectRef, handler)
			stored, ok := sources.For(v1alpha2.DataSourceTypeObjectRef)
			Expect(ok).To(BeTrue())
			Expect(stored).To(BeIdenticalTo(handler))
			Expect(sources.Changed(context.Background(), vi)).To(BeTrue())

			vi.Status.ObservedGeneration = 2
			Expect(sources.Changed(context.Background(), vi)).To(BeFalse())
		})

		It("aggregates cleanup results from all handlers", func() {
			sources := NewSources()
			first := &sourcesHandlerStub{}
			second := &sourcesHandlerStub{cleanupRequeue: true, cleanupReason: "waiting for cleanup"}
			sources.Set(v1alpha2.DataSourceTypeHTTP, first)
			sources.Set(v1alpha2.DataSourceTypeObjectRef, second)

			requeue, _, err := sources.CleanUp(context.Background(), newVI())
			Expect(err).ToNot(HaveOccurred())
			Expect(requeue).To(BeTrue())
			Expect(first.cleanupCalls).To(Equal(1))
			Expect(second.cleanupCalls).To(Equal(1))
		})

		It("returns cleanup error immediately", func() {
			sources := NewSources()
			broken := &sourcesHandlerStub{cleanupErr: errors.New("cleanup failed")}
			sources.Set(v1alpha2.DataSourceTypeHTTP, broken)

			requeue, _, err := sources.CleanUp(context.Background(), newVI())
			Expect(err).To(MatchError("cleanup failed"))
			Expect(requeue).To(BeFalse())
			Expect(broken.cleanupCalls).To(Equal(1))
		})
	})

	Describe("cleanup wrappers", func() {
		It("runs cleanup only when subresources should be deleted", func() {
			vi := newVI()
			cleaner := &sourcesCleanerStub{cleanupRequeue: true, cleanupReason: "waiting for cleanup"}

			requeue, _, err := CleanUp(context.Background(), vi, cleaner)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeue).To(BeTrue())
			Expect(cleaner.cleanupCalls).To(Equal(1))
		})

		It("skips cleanup when retain annotation is set", func() {
			vi := newVI()
			vi.Annotations[annotations.AnnPodRetainAfterCompletion] = "true"
			cleaner := &sourcesCleanerStub{cleanupRequeue: true, cleanupReason: "waiting for cleanup"}

			requeue, _, err := CleanUp(context.Background(), vi, cleaner)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeue).To(BeFalse())
			Expect(cleaner.cleanupCalls).To(BeZero())
		})

		It("runs supplements cleanup only when subresources should be deleted", func() {
			vi := newVI()
			cleaner := &sourcesCleanerStub{cleanupSupplementsResult: reconcile.Result{RequeueAfter: time.Second}}

			result, err := CleanUpSupplements(context.Background(), vi, cleaner)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Second))
			Expect(cleaner.cleanupSupplementsCalls).To(Equal(1))
		})

		It("skips supplements cleanup when retain annotation is set", func() {
			vi := newVI()
			vi.Annotations[annotations.AnnPodRetainAfterCompletion] = "true"
			cleaner := &sourcesCleanerStub{cleanupSupplementsResult: reconcile.Result{RequeueAfter: time.Second}}

			result, err := CleanUpSupplements(context.Background(), vi, cleaner)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(cleaner.cleanupSupplementsCalls).To(BeZero())
		})
	})

	It("detects finished image provisioning by ready reason", func() {
		Expect(IsImageProvisioningFinished(metav1.Condition{Reason: vicondition.Ready.String()})).To(BeTrue())
		Expect(IsImageProvisioningFinished(metav1.Condition{Reason: vicondition.Provisioning.String()})).To(BeFalse())
	})

	Describe("PVC import resume", func() {
		It("waits for populator to import an existing target PVC created from DVCR", func() {
			ctx := context.Background()
			vi := newVI()
			vi.Status.StorageClassName = "sc"
			pvc := newBoundImportPVC("target", vi.Namespace)
			client := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(pvc).Build()
			disk := service.NewDiskService(client, nil, nil, "vi-controller", service.DiskImporterConfig{Image: "pvc-importer", Verbose: "1"})
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
			source := service.NewPVCRegistryImportSource("docker://registry.example/image", "", "")
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			result, err := reconcilePVCImportFromDVCR(ctx, vi, &corev1.Pod{}, pvc, source, cb, supgen, nil, disk)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).ToNot(BeZero())

			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageProvisioning))
			Expect(cb.Condition().Reason).To(Equal(vicondition.Provisioning.String()))
		})

		It("waits for populator to import an existing target PVC created from another PVC", func() {
			ctx := context.Background()
			vi := newVI()
			vi.Status.StorageClassName = "sc"
			pvc := newBoundImportPVC("target", vi.Namespace)
			sourcePVC := newBoundImportPVC("source", vi.Namespace)
			client := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(pvc, sourcePVC).Build()
			disk := service.NewDiskService(client, nil, nil, "vi-controller", service.DiskImporterConfig{Image: "pvc-importer", Verbose: "1"})
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
			source := service.NewPVCPVCImportSource(sourcePVC.Name, sourcePVC.Namespace)
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			result, err := reconcilePVCImportFromReadySource(ctx, vi, pvc, source, resource.MustParse("1Gi"), cb, supgen, nil, disk, func() {})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).ToNot(BeZero())

			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageProvisioning))
			Expect(cb.Condition().Reason).To(Equal(vicondition.Provisioning.String()))
		})

		It("fails the image with the reason and keeps checking on the retrying importer", func() {
			// The importer pod belongs to the PersistentVolumeClaim, and the image only watches
			// pods it owns: dropping the requeue here would leave the image failed even after a
			// later attempt succeeds.
			ctx := context.Background()
			vi := newVI()
			vi.Status.StorageClassName = "sc"
			pvc := newBoundImportPVC("target", vi.Namespace)
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: supgen.PVCImporterPod().Name, Namespace: vi.Namespace},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{{
						LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
							Message: `{"error-message":"Unable to process data: no space left on device","permanent":true}`,
						}},
					}},
				},
			}
			client := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(pvc, pod).Build()
			disk := service.NewDiskService(client, nil, nil, "vi-controller", service.DiskImporterConfig{Image: "pvc-importer", Verbose: "1"})
			source := service.NewPVCRegistryImportSource("docker://registry.example/image", "", "")
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			result, err := reconcilePVCImportFromDVCR(ctx, vi, &corev1.Pod{}, pvc, source, cb, supgen, nil, disk)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).ToNot(BeZero())

			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
			Expect(cb.Condition().Reason).To(Equal(vicondition.ProvisioningFailed.String()))
			Expect(cb.Condition().Message).To(Equal("Unable to process data: no space left on device."))
		})

		It("fails provisioning when the source PVC lives on a different CSI driver", func() {
			ctx := context.Background()
			vi := newVI()
			vi.Status.StorageClassName = "dst-sc"
			sourcePVC := newBoundImportPVC("source", vi.Namespace)
			sourcePVC.Spec.StorageClassName = ptr.To("src-sc")
			srcSC := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "src-sc"},
				Provisioner: "src.csi.example.com",
			}
			dstSC := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "dst-sc"},
				Provisioner: "dst.csi.example.com",
			}
			client := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(sourcePVC, srcSC, dstSC).Build()
			disk := service.NewDiskService(client, nil, nil, "vi-controller", service.DiskImporterConfig{Image: "pvc-importer", Verbose: "1"})
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
			source := service.NewPVCPVCImportSource(sourcePVC.Name, sourcePVC.Namespace)
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			result, err := reconcilePVCImportFromReadySource(ctx, vi, nil, source, resource.MustParse("1Gi"), cb, supgen, nil, disk, func() {})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
			Expect(cb.Condition().Reason).To(Equal(vicondition.ProvisioningFailed.String()))
			Expect(cb.Condition().Message).To(ContainSubstring("Cross-provider PVC copy is not supported"))
		})

		It("sets raw format when block PVC import is complete", func() {
			ctx := context.Background()
			vi := newVI()
			pvc := newBoundImportPVC("target", vi.Namespace)
			pvc.Spec.VolumeMode = ptr.To(corev1.PersistentVolumeBlock)
			pvc.Annotations[annotations.AnnPVCPopulationDone] = "true"
			supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			result, err := reconcilePVCImportFromReadySource(ctx, vi, pvc, nil, resource.MustParse("200Mi"), cb, supgen, nil, nil, func() {})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).ToNot(BeZero())

			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageReady))
			Expect(vi.Status.Format).To(Equal(imageformat.FormatRAW))
		})
	})

	DescribeTable(
		"setPhaseConditionForFinishedImage",
		func(
			pvc *corev1.PersistentVolumeClaim,
			expectedPhase v1alpha2.ImagePhase,
			expectedStatus metav1.ConditionStatus,
			expectedReason string,
			expectedMessage string,
		) {
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)
			phase := v1alpha2.ImagePhase("")
			supgen := supplements.NewGenerator("vi", "image", "default", "uid")

			setPhaseConditionForFinishedImage(pvc, cb, &phase, supgen)

			Expect(phase).To(Equal(expectedPhase))
			Expect(cb.Condition().Status).To(Equal(expectedStatus))
			Expect(cb.Condition().Reason).To(Equal(expectedReason))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
		},
		Entry("marks pvc lost when pvc is missing", nil, v1alpha2.ImagePVCLost, metav1.ConditionFalse, vicondition.PVCLost.String(), `The underlying PersistentVolumeClaim "default/d8v-vi-image-uid" was not found.`),
		Entry("marks image ready when pvc exists", &corev1.PersistentVolumeClaim{}, v1alpha2.ImageReady, metav1.ConditionTrue, vicondition.Ready.String(), ""),
	)

	DescribeTable(
		"setPhaseConditionFromPodError",
		func(
			inputErr error,
			expectedErr error,
			expectedReason string,
			expectedMessage string,
		) {
			vi := newVI()
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			err := setPhaseConditionFromPodError(cb, vi, inputErr)
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}

			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
			Expect(cb.Condition().Reason).To(Equal(expectedReason))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
		},
		Entry("not initialized", servicestat.ErrNotInitialized, nil, vicondition.ProvisioningNotStarted.String(), "Not initialized."),
		Entry("not scheduled", servicestat.ErrNotScheduled, nil, vicondition.ProvisioningNotStarted.String(), "Not scheduled."),
		Entry("provisioning failed", servicestat.ErrProvisioningFailed, nil, vicondition.ProvisioningFailed.String(), "Provisioning failed."),
		Entry("unknown error", errors.New("boom"), errors.New("boom"), conditions.ReasonUnknown.String(), ""),
	)

	DescribeTable(
		"setPhaseConditionFromStorageError",
		func(
			inputErr error,
			expectedHandled bool,
			expectedErr error,
			expectedPhase v1alpha2.ImagePhase,
			expectedReason string,
			expectedMessage string,
		) {
			vi := newVI()
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			_, handled, err := setPhaseConditionFromStorageError(inputErr, vi, cb)
			Expect(handled).To(Equal(expectedHandled))
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}

			Expect(vi.Status.Phase).To(Equal(expectedPhase))
			Expect(cb.Condition().Reason).To(Equal(expectedReason))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
		},
		Entry("no error", nil, false, nil, v1alpha2.ImagePhase(""), conditions.ReasonUnknown.String(), ""),
		Entry("storage profile missing", volumemode.ErrStorageProfileNotFound, true, nil, v1alpha2.ImageFailed, vicondition.ProvisioningFailed.String(), "The StorageClass is not fully configured in the cluster. Check the StorageClass name or set a default StorageClass."),
		Entry("default storage class missing", service.ErrDefaultStorageClassNotFound, true, nil, v1alpha2.ImagePending, vicondition.ProvisioningFailed.String(), "Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass."),
		Entry(
			"quota exceeded",
			errors.New("create dvcr target pvc: exceeded quota: persistentvolumeclaims"),
			true,
			nil,
			v1alpha2.ImageFailed,
			vicondition.QuotaExceeded.String(),
			"Quota exceeded: create dvcr target pvc: exceeded quota: persistentvolumeclaims; Retry in 1 minute.",
		),
		Entry("unexpected error", errors.New("boom"), false, errors.New("boom"), v1alpha2.ImagePhase(""), conditions.ReasonUnknown.String(), ""),
	)

	It("propagates the quota retry instead of dropping it", func() {
		// The retry the quota branch asks for used to be discarded by the caller, so the image
		// waited for a reconcile that never came: nothing else wakes it while a quota blocks
		// the target claim.
		vi := newVI()
		vi.CreationTimestamp = metav1.NewTime(time.Now().Add(-31 * time.Minute))
		cb := conditions.NewConditionBuilder(vicondition.ReadyType)

		res, handled, err := setPhaseConditionFromStorageError(
			fmt.Errorf("create dvcr target pvc: %w", errors.New("exceeded quota: persistentvolumeclaims")), vi, cb)

		Expect(err).ToNot(HaveOccurred())
		Expect(handled).To(BeTrue())
		Expect(res.RequeueAfter).To(Equal(time.Minute))
	})

	DescribeTable(
		"setQuotaExceededPhaseCondition",
		func(
			creationTimestamp metav1.Time,
			expectedMessage string,
			expectedRequeueAfter time.Duration,
		) {
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)
			phase := v1alpha2.ImagePhase("")

			result := setQuotaExceededPhaseCondition(cb, &phase, errors.New("exceeded quota: test"), creationTimestamp)
			Expect(phase).To(Equal(v1alpha2.ImageFailed))
			Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
			Expect(cb.Condition().Reason).To(Equal(vicondition.QuotaExceeded.String()))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
			Expect(result.RequeueAfter).To(Equal(expectedRequeueAfter))
		},
		Entry("keeps failed state for fresh object", metav1.NewTime(time.Now()), "Quota exceeded: exceeded quota: test; Please configure quotas or try recreating the resource later.", time.Duration(0)),
		Entry("requeues old object", metav1.NewTime(time.Now().Add(-31*time.Minute)), "Quota exceeded: exceeded quota: test; Retry in 1 minute.", time.Minute),
	)
})

var _ = Describe("pvcImportFailed", func() {
	newPod := func(state, last corev1.ContainerState, phase corev1.PodPhase) *corev1.Pod {
		return &corev1.Pod{
			Status: corev1.PodStatus{
				Phase:             phase,
				ContainerStatuses: []corev1.ContainerStatus{{State: state, LastTerminationState: last}},
			},
		}
	}

	permanentReport := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
		Message: `{"error-message":"Unable to process data: no space left on device","permanent":true}`,
	}}

	take := func(pod *corev1.Pod) (*v1alpha2.VirtualImage, bool) {
		vi := &v1alpha2.VirtualImage{Status: v1alpha2.VirtualImageStatus{Phase: v1alpha2.ImageProvisioning}}
		cb := conditions.NewConditionBuilder(vicondition.ReadyType)
		failed := pvcImportFailed(vi, cb, pod)
		conditions.SetCondition(cb, &vi.Status.Conditions)
		return vi, failed
	}

	It("keeps provisioning when the importer pod is not there", func() {
		_, failed := take(nil)
		Expect(failed).To(BeFalse())
	})

	It("keeps provisioning while a new attempt is running", func() {
		_, failed := take(newPod(corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}, permanentReport, corev1.PodRunning))
		Expect(failed).To(BeFalse())
	})

	It("fails the image with the reason the importer reported", func() {
		// The pvc-importer pod runs with RestartPolicy: OnFailure and never reaches PodFailed,
		// so without the report the image would report "importing" forever.
		vi, failed := take(newPod(corev1.ContainerState{}, permanentReport, corev1.PodRunning))
		Expect(failed).To(BeTrue())
		Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
		ready, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
		Expect(ready.Message).To(Equal("Unable to process data: no space left on device."))
	})

	It("keeps provisioning while the failure may still go away", func() {
		transient := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Message: `{"error-message":"failed to pull image: connection refused"}`,
		}}
		_, failed := take(newPod(corev1.ContainerState{}, transient, corev1.PodRunning))
		Expect(failed).To(BeFalse())
	})

	It("keeps a finished import alive when an earlier attempt had failed", func() {
		// The successful attempt terminates the container with "Import Complete"; the failed
		// one before it stays in LastTerminationState and must not outweigh it.
		succeeded := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
			Message: `{"source-image-size":1,"message":"Import Complete"}`,
		}}
		_, failed := take(newPod(succeeded, permanentReport, corev1.PodSucceeded))
		Expect(failed).To(BeFalse())
	})

	It("fails the image when the pod itself failed", func() {
		vi, failed := take(newPod(corev1.ContainerState{}, corev1.ContainerState{}, corev1.PodFailed))
		Expect(failed).To(BeTrue())
		Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
		ready, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
		Expect(ready.Message).To(Equal("VirtualImage importer Pod failed."))
	})
})

var _ = Describe("refreshPVCImportProgress", func() {
	It("republishes the progress of the pod it was given", func() {
		// The pod is fetched once per reconcile and handed around; the progress must still be
		// taken from that same pod.
		var seen string
		statSvc := &StatMock{
			GetProgressFunc: func(_ types.UID, pod *corev1.Pod, _ string, _ ...servicestat.GetProgressOption) string {
				seen = pod.Name
				return "42%"
			},
		}
		vi := &v1alpha2.VirtualImage{}

		refreshPVCImportProgress(vi, statSvc, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "importer"}}, nil)

		Expect(seen).To(Equal("importer"))
		Expect(vi.Status.Progress).To(Equal("42%"))
	})

	It("keeps the previous progress when there is no pod", func() {
		statSvc := &StatMock{
			GetProgressFunc: func(_ types.UID, _ *corev1.Pod, _ string, _ ...servicestat.GetProgressOption) string {
				Fail("the stat service must not be asked about a pod that is not there")
				return ""
			},
		}
		vi := &v1alpha2.VirtualImage{Status: v1alpha2.VirtualImageStatus{Progress: "17%"}}

		refreshPVCImportProgress(vi, statSvc, nil, nil)

		Expect(vi.Status.Progress).To(Equal("17%"))
	})
})
