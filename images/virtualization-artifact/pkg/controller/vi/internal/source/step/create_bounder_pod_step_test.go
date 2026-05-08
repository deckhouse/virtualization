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

package step

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type createBounderPodStepBounderStub struct {
	startErr   error
	startCalls int
}

func (s *createBounderPodStepBounderStub) Start(_ context.Context, _ *metav1.OwnerReference, _ supplements.Generator, _ ...service.Option) error {
	s.startCalls++
	return s.startErr
}

var _ = Describe("CreateBounderPodStep", func() {
	DescribeTable("Take",
		func(
			pvc *corev1.PersistentVolumeClaim,
			storageClasses []client.Object,
			bounderErr error,
			creationTimestamp metav1.Time,
			expectedErr error,
			expectedStartCalls int,
			expectedEventCalls int,
			expectedPhase v1alpha2.ImagePhase,
			expectedReason string,
			expectedMessage string,
			expectedRequeueAfter time.Duration,
		) {
			scheme := runtime.NewScheme()
			Expect(storagev1.AddToScheme(scheme)).To(Succeed())

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(storageClasses...).Build()
			bounder := &createBounderPodStepBounderStub{startErr: bounderErr}
			var recorder *eventrecord.EventRecorderLoggerMock
			recorder = &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(client.Object, string, string, string) {},
				WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
					return recorder
				},
			}
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)
			vi := &v1alpha2.VirtualImage{
				TypeMeta: metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: "VirtualImage"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "vi",
					Namespace:         "default",
					UID:               "vi-uid",
					CreationTimestamp: creationTimestamp,
				},
			}

			result, err := NewCreateBounderPodStep(pvc, bounder, fakeClient, recorder, cb).Take(context.Background(), vi)
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}

			Expect(bounder.startCalls).To(Equal(expectedStartCalls))
			Expect(recorder.EventCalls()).To(HaveLen(expectedEventCalls))

			if result == nil {
				Expect(expectedRequeueAfter).To(BeZero())
			} else {
				Expect(result.RequeueAfter).To(Equal(expectedRequeueAfter))
			}

			Expect(vi.Status.Phase).To(Equal(expectedPhase))
			Expect(cb.Condition().Reason).To(Equal(expectedReason))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
			if expectedReason != conditions.ReasonUnknown.String() {
				Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
			}

			if expectedEventCalls > 0 {
				event := recorder.EventCalls()[0]
				Expect(event.Eventtype).To(Equal(corev1.EventTypeWarning))
				Expect(event.Reason).To(Equal(v1alpha2.ReasonDataSourceQuotaExceeded))
				Expect(event.Message).To(Equal("DataSource quota exceed"))
			}
		},
		Entry("skips when pvc is absent",
			nil,
			nil,
			nil,
			metav1.NewTime(time.Now()),
			nil,
			0,
			0,
			v1alpha2.ImagePhase(""),
			conditions.ReasonUnknown.String(),
			"",
			time.Duration(0),
		),
		Entry("skips when pvc has no storage class",
			&corev1.PersistentVolumeClaim{},
			nil,
			nil,
			metav1.NewTime(time.Now()),
			nil,
			0,
			0,
			v1alpha2.ImagePhase(""),
			conditions.ReasonUnknown.String(),
			"",
			time.Duration(0),
		),
		Entry("skips for non wait for first consumer storage class",
			newPVCWithStorageClass("immediate"),
			[]client.Object{newStorageClass("immediate", storagev1.VolumeBindingImmediate)},
			nil,
			metav1.NewTime(time.Now()),
			nil,
			0,
			0,
			v1alpha2.ImagePhase(""),
			conditions.ReasonUnknown.String(),
			"",
			time.Duration(0),
		),
		Entry("starts bounder for wait for first consumer storage class",
			newPVCWithStorageClass("wffc"),
			[]client.Object{newStorageClass("wffc", storagev1.VolumeBindingWaitForFirstConsumer)},
			nil,
			metav1.NewTime(time.Now()),
			nil,
			1,
			0,
			v1alpha2.ImagePhase(""),
			vicondition.Provisioning.String(),
			"Bounder pod has created: waiting to be Bound.",
			time.Duration(0),
		),
		Entry("handles quota exceeded error",
			newPVCWithStorageClass("wffc"),
			[]client.Object{newStorageClass("wffc", storagev1.VolumeBindingWaitForFirstConsumer)},
			errors.New("exceeded quota: test quota"),
			metav1.NewTime(time.Now()),
			nil,
			1,
			1,
			v1alpha2.ImageFailed,
			vicondition.ProvisioningFailed.String(),
			"Quota exceeded: exceeded quota: test quota; Please configure quotas or try recreating the resource later.",
			time.Duration(0),
		),
		Entry("returns unknown bounder error",
			newPVCWithStorageClass("wffc"),
			[]client.Object{newStorageClass("wffc", storagev1.VolumeBindingWaitForFirstConsumer)},
			errors.New("boom"),
			metav1.NewTime(time.Now()),
			errors.New("boom"),
			1,
			0,
			v1alpha2.ImageFailed,
			vicondition.ProvisioningFailed.String(),
			"Unexpected error: boom",
			time.Duration(0),
		),
	)
})

func newPVCWithStorageClass(name string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &name,
		},
	}
}

func newStorageClass(name string, mode storagev1.VolumeBindingMode) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: name},
		VolumeBindingMode: ptr.To(mode),
	}
}
