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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	servicestat "github.com/deckhouse/virtualization-controller/pkg/controller/service/stat"
	serviceuploader "github.com/deckhouse/virtualization-controller/pkg/controller/service/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

var _ = Describe("Upload DataSource", func() {
	var (
		ctx            context.Context
		cvi            *v1alpha2.ClusterVirtualImage
		pod            *corev1.Pod
		svc            *corev1.Service
		exposure       serviceuploader.UploaderExposure
		uploaderMock   *UploaderMock
		statMock       *StatMock
		recordedEvents []string
		ds             *UploadDataSource
	)

	newUploadDataSource := func() *UploadDataSource {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, reason, _ string) {
				recordedEvents = append(recordedEvents, reason)
			},
		}
		return NewUploadDataSource(
			recorder,
			statMock,
			uploaderMock,
			&dvcr.Settings{},
			"controller-ns",
			fake.NewClientBuilder().WithScheme(scheme).Build(),
		)
	}

	BeforeEach(func() {
		ctx = context.Background()
		recordedEvents = nil

		cvi = &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "cvi",
				Generation: 1,
				UID:        "11111111-1111-1111-1111-111111111111",
			},
			Spec: v1alpha2.ClusterVirtualImageSpec{
				DataSource: v1alpha2.ClusterVirtualImageDataSource{
					Type: v1alpha2.DataSourceTypeUpload,
				},
			},
			Status: v1alpha2.ClusterVirtualImageStatus{
				ImageUploadURLs: &v1alpha2.ImageUploadURLs{External: "https://upload.example.com"},
			},
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "uploader",
				CreationTimestamp: metav1.NewTime(time.Now()),
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
		svc = &corev1.Service{}
		exposure = serviceuploader.UploaderExposure{
			Exists:    true,
			UploadURL: "https://upload.example.com",
		}

		uploaderMock = &UploaderMock{
			GetPodFunc: func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) {
				return pod, nil
			},
			GetServiceFunc: func(_ context.Context, _ supplements.Generator) (*corev1.Service, error) {
				return svc, nil
			},
			GetExposureFunc: func(_ context.Context, _ supplements.Generator) (serviceuploader.UploaderExposure, error) {
				return exposure, nil
			},
			EnsureExposureFunc: func(_ context.Context, _ client.Object, _ supplements.Generator) error {
				return nil
			},
			CleanupFunc: func(_ context.Context, _ supplements.Generator) (bool, string, error) {
				return true, "", nil
			},
			GetInClusterURLFunc: func(_ *corev1.Service) string {
				return "http://10.0.0.1/upload"
			},
		}
		statMock = &StatMock{
			IsUploaderReadyFunc: func(_ *corev1.Pod, _ *corev1.Service, _ serviceuploader.UploaderExposure) (bool, error) {
				return true, nil
			},
			IsUploadStartedFunc: func(_ types.UID, _ *corev1.Pod) bool {
				return false
			},
			GetDVCRImageNameFunc: func(_ *corev1.Pod) string {
				return "registry.example.com/image:test"
			},
		}
	})

	It("waits for the user upload while the timeout has not expired", func() {
		ds = newUploadDataSource()

		res, err := ds.Sync(ctx, cvi)

		Expect(err).NotTo(HaveOccurred())
		// The start of the upload is only visible through the pod metrics scraped on
		// reconcile, so waiting has to keep polling.
		Expect(res.RequeueAfter).To(Equal(time.Second))
		Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageWaitForUserUpload))
		ready, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
		Expect(ready.Reason).To(Equal(cvicondition.WaitForUserUpload.String()))
		Expect(uploaderMock.CleanupCalls()).To(BeEmpty())
	})

	It("polls every second while the upload is in progress", func() {
		statMock.IsUploadStartedFunc = func(_ types.UID, _ *corev1.Pod) bool {
			return true
		}
		statMock.CheckPodFunc = func(_ *corev1.Pod) error {
			return nil
		}
		statMock.GetProgressFunc = func(_ types.UID, _ *corev1.Pod, _ string, _ ...servicestat.GetProgressOption) string {
			return "42%"
		}
		statMock.GetDownloadSpeedFunc = func(_ types.UID, _ *corev1.Pod) *v1alpha2.StatusSpeed {
			return &v1alpha2.StatusSpeed{Current: "1Mbps"}
		}
		ds = newUploadDataSource()

		res, err := ds.Sync(ctx, cvi)

		Expect(err).NotTo(HaveOccurred())
		// Progress is only refreshed on reconcile, so the poll has to stay tight.
		Expect(res.RequeueAfter).To(Equal(time.Second))
		Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageProvisioning))
		Expect(cvi.Status.Progress).To(Equal("42%"))
		Expect(cvi.Status.DownloadSpeed).ToNot(BeNil())
	})

	It("fails the import when the upload has not started within the timeout", func() {
		cvi.Status.Conditions = []metav1.Condition{{
			Type:               cvicondition.ReadyType.String(),
			Status:             metav1.ConditionFalse,
			Reason:             cvicondition.WaitForUserUpload.String(),
			LastTransitionTime: metav1.NewTime(time.Now().Add(-serviceuploader.WaitForUserUploadTimeout - time.Minute)),
		}}
		ds = newUploadDataSource()

		res, err := ds.Sync(ctx, cvi)

		Expect(err).NotTo(HaveOccurred())
		Expect(res.IsZero()).To(BeTrue())
		Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
		Expect(cvi.Status.ImageUploadURLs).To(BeNil())
		ready, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(cvicondition.WaitForUserUploadTimeout.String()))
		Expect(ready.Message).To(Equal(serviceuploader.WaitForUserUploadTimeoutMessage("ClusterVirtualImage")))
		Expect(uploaderMock.CleanupCalls()).To(HaveLen(1))
		Expect(recordedEvents).To(ContainElement(v1alpha2.ReasonDataSourceSyncFailed))
	})

	It("keeps the failed state and does not recreate the uploader after the timeout", func() {
		cvi.Status.Phase = v1alpha2.ImageFailed
		cvi.Status.Conditions = []metav1.Condition{{
			Type:   cvicondition.ReadyType.String(),
			Status: metav1.ConditionFalse,
			Reason: cvicondition.WaitForUserUploadTimeout.String(),
		}}
		uploaderMock.GetPodFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) {
			return nil, nil
		}
		uploaderMock.GetServiceFunc = func(_ context.Context, _ supplements.Generator) (*corev1.Service, error) {
			return nil, nil
		}
		uploaderMock.GetExposureFunc = func(_ context.Context, _ supplements.Generator) (serviceuploader.UploaderExposure, error) {
			return serviceuploader.UploaderExposure{}, nil
		}
		statMock.IsUploaderReadyFunc = func(_ *corev1.Pod, _ *corev1.Service, _ serviceuploader.UploaderExposure) (bool, error) {
			return false, nil
		}
		ds = newUploadDataSource()

		res, err := ds.Sync(ctx, cvi)

		Expect(err).NotTo(HaveOccurred())
		Expect(res.IsZero()).To(BeTrue())
		Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
		ready, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
		Expect(ready.Reason).To(Equal(cvicondition.WaitForUserUploadTimeout.String()))
		// Apply must not be called: ApplyFunc is nil and would panic.
		Expect(uploaderMock.ApplyCalls()).To(BeEmpty())
	})
})
