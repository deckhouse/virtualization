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

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	servicestat "github.com/deckhouse/virtualization-controller/pkg/controller/service/stat"
	serviceuploader "github.com/deckhouse/virtualization-controller/pkg/controller/service/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("Upload DataSource StoreToDVCR", func() {
	var (
		ctx          context.Context
		vi           *v1alpha2.VirtualImage
		pod          *corev1.Pod
		svc          *corev1.Service
		exposure     serviceuploader.UploaderExposure
		uploaderMock *UploaderMock
		statMock     *StatMock
	)

	newUploadDataSource := func() *UploadDataSource {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		return NewUploadDataSource(
			&eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, _, _, _ string) {},
			},
			statMock,
			uploaderMock,
			&dvcr.Settings{},
			nil,
			fake.NewClientBuilder().WithScheme(scheme).Build(),
		)
	}

	BeforeEach(func() {
		ctx = context.Background()

		vi = &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "vi",
				Namespace:  "default",
				Generation: 1,
				UID:        "22222222-2222-2222-2222-222222222222",
			},
			Spec: v1alpha2.VirtualImageSpec{
				DataSource: v1alpha2.VirtualImageDataSource{
					Type: v1alpha2.DataSourceTypeUpload,
				},
			},
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "uploader", Namespace: vi.Namespace},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		svc = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "uploader-svc", Namespace: vi.Namespace}}
		exposure = serviceuploader.UploaderExposure{
			Required:  true,
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
			CheckPodFunc: func(_ *corev1.Pod) error {
				return nil
			},
			GetDVCRImageNameFunc: func(_ *corev1.Pod) string {
				return "registry.example.com/image:test"
			},
		}
	})

	It("keeps polling while waiting for the user upload", func() {
		res, err := newUploadDataSource().StoreToDVCR(ctx, vi)

		Expect(err).ToNot(HaveOccurred())
		Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageWaitForUserUpload))
		// The start of the upload is only visible through the pod metrics scraped on
		// reconcile: sleeping until the wait window expires would miss the whole upload.
		Expect(res.RequeueAfter).To(Equal(time.Second))
	})

	It("publishes the in-cluster URL only when no external exposure is expected", func() {
		// A cluster without publicDomainTemplate: nothing publishes the upload
		// externally, so the uploader must not be recreated on every reconcile just
		// because the exposure is missing.
		exposure = serviceuploader.UploaderExposure{Required: false}
		uploaderMock.ApplyFunc = func(_ context.Context, _ client.Object, _ supplements.Generator, _ serviceuploader.Settings, _ *datasource.CABundle, _ ...serviceuploader.Option) error {
			return nil
		}

		res, err := newUploadDataSource().StoreToDVCR(ctx, vi)

		Expect(err).ToNot(HaveOccurred())
		Expect(uploaderMock.ApplyCalls()).To(BeEmpty())
		Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageWaitForUserUpload))
		Expect(vi.Status.ImageUploadURLs).ToNot(BeNil())
		Expect(vi.Status.ImageUploadURLs.External).To(BeEmpty())
		Expect(vi.Status.ImageUploadURLs.InCluster).To(Equal("http://10.0.0.1/upload"))
		Expect(res.RequeueAfter).To(Equal(time.Second))
	})

	It("picks up the upload that started while waiting", func() {
		vi.Status.Conditions = []metav1.Condition{{
			Type:               vicondition.ReadyType.String(),
			Status:             metav1.ConditionFalse,
			Reason:             vicondition.WaitForUserUpload.String(),
			LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Minute)),
		}}
		statMock.IsUploadStartedFunc = func(_ types.UID, _ *corev1.Pod) bool {
			return true
		}
		statMock.GetProgressFunc = func(_ types.UID, _ *corev1.Pod, _ string, _ ...servicestat.GetProgressOption) string {
			return "17%"
		}
		statMock.GetDownloadSpeedFunc = func(_ types.UID, _ *corev1.Pod) *v1alpha2.StatusSpeed {
			return &v1alpha2.StatusSpeed{Current: "1Mbps"}
		}

		res, err := newUploadDataSource().StoreToDVCR(ctx, vi)

		Expect(err).ToNot(HaveOccurred())
		Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageProvisioning))
		Expect(vi.Status.Progress).To(Equal("17%"))
		Expect(res.RequeueAfter).To(Equal(time.Second))
	})

	It("polls every second while the upload is in progress", func() {
		statMock.IsUploadStartedFunc = func(_ types.UID, _ *corev1.Pod) bool {
			return true
		}
		statMock.GetProgressFunc = func(_ types.UID, _ *corev1.Pod, _ string, _ ...servicestat.GetProgressOption) string {
			return "42%"
		}
		statMock.GetDownloadSpeedFunc = func(_ types.UID, _ *corev1.Pod) *v1alpha2.StatusSpeed {
			return &v1alpha2.StatusSpeed{Current: "1Mbps"}
		}

		res, err := newUploadDataSource().StoreToDVCR(ctx, vi)

		Expect(err).ToNot(HaveOccurred())
		// Progress is only refreshed on reconcile, so the poll has to stay tight.
		Expect(res.RequeueAfter).To(Equal(time.Second))
		Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageProvisioning))
		Expect(vi.Status.Progress).To(Equal("42%"))
		Expect(vi.Status.DownloadSpeed).ToNot(BeNil())
	})
})
