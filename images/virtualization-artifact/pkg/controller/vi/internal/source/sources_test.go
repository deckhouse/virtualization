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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type sourcesHandlerStub struct {
	cleanupResult bool
	cleanupErr    error
	cleanupCalls  int
}

func (s *sourcesHandlerStub) StoreToDVCR(context.Context, *v1alpha2.VirtualImage) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (s *sourcesHandlerStub) StoreToPVC(context.Context, *v1alpha2.VirtualImage) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (s *sourcesHandlerStub) CleanUp(context.Context, *v1alpha2.VirtualImage) (bool, error) {
	s.cleanupCalls++
	return s.cleanupResult, s.cleanupErr
}

func (s *sourcesHandlerStub) Validate(context.Context, *v1alpha2.VirtualImage) error {
	return nil
}

type sourcesCleanerStub struct {
	cleanupResult            bool
	cleanupErr               error
	cleanupSupplementsResult reconcile.Result
	cleanupSupplementsErr    error
	cleanupCalls             int
	cleanupSupplementsCalls  int
}

func (s *sourcesCleanerStub) CleanUp(context.Context, *v1alpha2.VirtualImage) (bool, error) {
	s.cleanupCalls++
	return s.cleanupResult, s.cleanupErr
}

func (s *sourcesCleanerStub) CleanUpSupplements(context.Context, *v1alpha2.VirtualImage) (reconcile.Result, error) {
	s.cleanupSupplementsCalls++
	return s.cleanupSupplementsResult, s.cleanupSupplementsErr
}

type sourcesImportCheckerStub struct {
	err error
}

func (s sourcesImportCheckerStub) CheckImportProcess(context.Context, *cdiv1.DataVolume, *corev1.PersistentVolumeClaim) error {
	return s.err
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
			first := &sourcesHandlerStub{cleanupResult: false}
			second := &sourcesHandlerStub{cleanupResult: true}
			sources.Set(v1alpha2.DataSourceTypeHTTP, first)
			sources.Set(v1alpha2.DataSourceTypeObjectRef, second)

			requeue, err := sources.CleanUp(context.Background(), newVI())
			Expect(err).ToNot(HaveOccurred())
			Expect(requeue).To(BeTrue())
			Expect(first.cleanupCalls).To(Equal(1))
			Expect(second.cleanupCalls).To(Equal(1))
		})

		It("returns cleanup error immediately", func() {
			sources := NewSources()
			broken := &sourcesHandlerStub{cleanupErr: errors.New("cleanup failed")}
			sources.Set(v1alpha2.DataSourceTypeHTTP, broken)

			requeue, err := sources.CleanUp(context.Background(), newVI())
			Expect(err).To(MatchError("cleanup failed"))
			Expect(requeue).To(BeFalse())
			Expect(broken.cleanupCalls).To(Equal(1))
		})
	})

	Describe("cleanup wrappers", func() {
		It("runs cleanup only when subresources should be deleted", func() {
			vi := newVI()
			cleaner := &sourcesCleanerStub{cleanupResult: true}

			shouldRequeue, err := CleanUp(context.Background(), vi, cleaner)
			Expect(err).ToNot(HaveOccurred())
			Expect(shouldRequeue).To(BeTrue())
			Expect(cleaner.cleanupCalls).To(Equal(1))
		})

		It("skips cleanup when retain annotation is set", func() {
			vi := newVI()
			vi.Annotations[annotations.AnnPodRetainAfterCompletion] = "true"
			cleaner := &sourcesCleanerStub{cleanupResult: true}

			shouldRequeue, err := CleanUp(context.Background(), vi, cleaner)
			Expect(err).ToNot(HaveOccurred())
			Expect(shouldRequeue).To(BeFalse())
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
		Entry("marks pvc lost when pvc is missing", nil, v1alpha2.ImagePVCLost, metav1.ConditionFalse, vicondition.PVCLost.String(), "PVC default/d8v-vi-image-uid not found."),
		Entry("marks image ready when pvc exists", &corev1.PersistentVolumeClaim{}, v1alpha2.ImageReady, metav1.ConditionTrue, vicondition.Ready.String(), ""),
	)

	DescribeTable(
		"setPhaseConditionForPVCProvisioningImage",
		func(
			dv *cdiv1.DataVolume,
			checkerErr error,
			expectedPhase v1alpha2.ImagePhase,
			expectedStatus metav1.ConditionStatus,
			expectedReason string,
			expectedMessage string,
			expectedErr error,
		) {
			vi := newVI()
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			err := setPhaseConditionForPVCProvisioningImage(context.Background(), dv, vi, nil, cb, sourcesImportCheckerStub{err: checkerErr})
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}

			Expect(vi.Status.Phase).To(Equal(expectedPhase))
			Expect(cb.Condition().Status).To(Equal(expectedStatus))
			Expect(cb.Condition().Reason).To(Equal(expectedReason))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
		},
		Entry("waits for pvc importer creation when dv is absent", nil, nil, v1alpha2.ImageProvisioning, metav1.ConditionFalse, vicondition.Provisioning.String(), "Waiting for the pvc importer to be created", nil),
		Entry("reports provisioning in progress", &cdiv1.DataVolume{}, nil, v1alpha2.ImageProvisioning, metav1.ConditionFalse, vicondition.Provisioning.String(), "Import is in the process of provisioning to PVC.", nil),
		Entry("handles data volume not running", &cdiv1.DataVolume{}, service.ErrDataVolumeNotRunning, v1alpha2.ImageProvisioning, metav1.ConditionFalse, vicondition.ProvisioningFailed.String(), "Pvc importer is not running", nil),
		Entry("handles missing default storage class", &cdiv1.DataVolume{}, service.ErrDefaultStorageClassNotFound, v1alpha2.ImagePending, metav1.ConditionFalse, vicondition.ProvisioningFailed.String(), "Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass.", nil),
		Entry("returns unexpected error", &cdiv1.DataVolume{}, errors.New("boom"), v1alpha2.ImagePhase(""), metav1.ConditionUnknown, conditions.ReasonUnknown.String(), "", errors.New("boom")),
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
		Entry("not initialized", service.ErrNotInitialized, nil, vicondition.ProvisioningNotStarted.String(), "Not initialized."),
		Entry("not scheduled", service.ErrNotScheduled, nil, vicondition.ProvisioningNotStarted.String(), "Not scheduled."),
		Entry("provisioning failed", service.ErrProvisioningFailed, nil, vicondition.ProvisioningFailed.String(), "Provisioning failed."),
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

			handled, err := setPhaseConditionFromStorageError(inputErr, vi, cb)
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
		Entry("storage profile missing", volumemode.ErrStorageProfileNotFound, true, nil, v1alpha2.ImageFailed, vicondition.ProvisioningFailed.String(), "StorageProfile not found in the cluster: Please check a StorageClass name in the cluster or set a default StorageClass."),
		Entry("default storage class missing", service.ErrDefaultStorageClassNotFound, true, nil, v1alpha2.ImagePending, vicondition.ProvisioningFailed.String(), "Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass."),
		Entry("unexpected error", errors.New("boom"), false, errors.New("boom"), v1alpha2.ImagePhase(""), conditions.ReasonUnknown.String(), ""),
	)

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
			Expect(cb.Condition().Reason).To(Equal(vicondition.ProvisioningFailed.String()))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
			Expect(result.RequeueAfter).To(Equal(expectedRequeueAfter))
		},
		Entry("keeps failed state for fresh object", metav1.NewTime(time.Now()), "Quota exceeded: exceeded quota: test; Please configure quotas or try recreating the resource later.", time.Duration(0)),
		Entry("requeues old object", metav1.NewTime(time.Now().Add(-31*time.Minute)), "Quota exceeded: exceeded quota: test; Retry in 1 minute.", time.Minute),
	)
})
