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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type readyContainerRegistryStepCleanerStub struct {
	cleanupErr error
	calls      int
}

func (s *readyContainerRegistryStepCleanerStub) CleanUpSupplements(context.Context, supplements.Generator) (bool, error) {
	s.calls++
	if s.cleanupErr != nil {
		return false, s.cleanupErr
	}

	return true, nil
}

type readyContainerRegistryStepStatStub struct {
	checkPodErr   error
	size          v1alpha2.ImageStatusSize
	dvcrImageName string
	format        string
	cdrom         bool
}

func (s readyContainerRegistryStepStatStub) GetSize(_ *corev1.Pod) v1alpha2.ImageStatusSize {
	return s.size
}

func (s readyContainerRegistryStepStatStub) GetDVCRImageName(_ *corev1.Pod) string {
	return s.dvcrImageName
}

func (s readyContainerRegistryStepStatStub) GetFormat(_ *corev1.Pod) string {
	return s.format
}

func (s readyContainerRegistryStepStatStub) CheckPod(_ *corev1.Pod) error {
	return s.checkPodErr
}

func (s readyContainerRegistryStepStatStub) GetCDROM(_ *corev1.Pod) bool {
	return s.cdrom
}

var _ = Describe("ReadyContainerRegistryStep", func() {
	newRecorder := func() *eventrecord.EventRecorderLoggerMock {
		var recorder *eventrecord.EventRecorderLoggerMock
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc: noopEvent,
			WithLoggingFunc: func(eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
				return recorder
			},
		}

		return recorder
	}

	newVI := func() *v1alpha2.VirtualImage {
		return &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vi",
				Namespace: "default",
				UID:       types.UID("vi-uid"),
			},
		}
	}

	It("marks image ready immediately when ready condition is already true", func() {
		vi := newVI()
		vi.Status.Conditions = []metav1.Condition{{
			Type:   vicondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		}}
		cb := conditions.NewConditionBuilder(vicondition.ReadyType)
		importer := &readyContainerRegistryStepCleanerStub{}
		diskService := &readyContainerRegistryStepCleanerStub{}

		result, err := NewReadyContainerRegistryStep(nil, diskService, importer, readyContainerRegistryStepStatStub{}, newRecorder(), cb).Take(context.Background(), vi)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(*result).To(Equal(reconcile.Result{}))
		Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageReady))
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionTrue))
		Expect(cb.Condition().Reason).To(Equal(vicondition.Ready.String()))
		Expect(cb.Condition().Message).To(BeEmpty())
		Expect(importer.calls).To(BeZero())
		Expect(diskService.calls).To(BeZero())
	})

	It("waits while pod is not complete", func() {
		vi := newVI()
		cb := conditions.NewConditionBuilder(vicondition.ReadyType)
		importer := &readyContainerRegistryStepCleanerStub{}
		diskService := &readyContainerRegistryStepCleanerStub{}
		pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}

		result, err := NewReadyContainerRegistryStep(pod, diskService, importer, readyContainerRegistryStepStatStub{}, newRecorder(), cb).Take(context.Background(), vi)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(vi.Status.Phase).To(BeZero())
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionUnknown))
		Expect(cb.Condition().Reason).To(Equal(conditions.ReasonUnknown.String()))
		Expect(importer.calls).To(BeZero())
		Expect(diskService.calls).To(BeZero())
	})

	DescribeTable(
		"handles completed pod results",
		func(
			checkPodErr error,
			expectedErr error,
			expectedPhase v1alpha2.ImagePhase,
			expectedReason string,
			expectedMessage string,
		) {
			vi := newVI()
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)
			pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}

			result, err := NewReadyContainerRegistryStep(
				pod,
				&readyContainerRegistryStepCleanerStub{},
				&readyContainerRegistryStepCleanerStub{},
				readyContainerRegistryStepStatStub{checkPodErr: checkPodErr},
				newRecorder(),
				cb,
			).Take(context.Background(), vi)

			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(*result).To(Equal(reconcile.Result{}))
			} else {
				Expect(err).To(MatchError(expectedErr))
				Expect(result).To(BeNil())
			}

			Expect(vi.Status.Phase).To(Equal(expectedPhase))
			Expect(cb.Condition().Reason).To(Equal(expectedReason))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
			if expectedReason == vicondition.ProvisioningFailed.String() {
				Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
			} else {
				Expect(cb.Condition().Status).To(Equal(metav1.ConditionUnknown))
			}
		},
		Entry("sets provisioning failed condition", fmt.Errorf("%w: importer failed", service.ErrProvisioningFailed), nil,
			v1alpha2.ImageFailed,
			vicondition.ProvisioningFailed.String(),
			"Provisioning failed: importer failed.",
		),
		Entry("returns unknown error", errors.New("boom"), errors.New("boom"),
			v1alpha2.ImageFailed,
			conditions.ReasonUnknown.String(),
			"",
		),
	)

	DescribeTable(
		"returns cleanup errors",
		func(
			importerErr error,
			diskServiceErr error,
			expectedErr string,
			expectedImporterCalls int,
			expectedDiskServiceCalls int,
		) {
			vi := newVI()
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)
			pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}
			importer := &readyContainerRegistryStepCleanerStub{cleanupErr: importerErr}
			diskService := &readyContainerRegistryStepCleanerStub{cleanupErr: diskServiceErr}

			result, err := NewReadyContainerRegistryStep(
				pod,
				diskService,
				importer,
				readyContainerRegistryStepStatStub{},
				newRecorder(),
				cb,
			).Take(context.Background(), vi)

			Expect(err).To(MatchError(expectedErr))
			Expect(result).To(BeNil())
			Expect(importer.calls).To(Equal(expectedImporterCalls))
			Expect(diskService.calls).To(Equal(expectedDiskServiceCalls))
			Expect(cb.Condition().Status).To(Equal(metav1.ConditionUnknown))
		},
		Entry("when importer cleanup fails", errors.New("importer cleanup failed"), nil,
			"clean up supplements: importer cleanup failed", 1, 0,
		),
		Entry("when disk service cleanup fails", nil, errors.New("disk service cleanup failed"),
			"clean up supplements: disk service cleanup failed", 1, 1,
		),
	)

	It("marks image ready and fills status fields after successful cleanup", func() {
		vi := newVI()
		cb := conditions.NewConditionBuilder(vicondition.ReadyType)
		pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}}
		importer := &readyContainerRegistryStepCleanerStub{}
		diskService := &readyContainerRegistryStepCleanerStub{}
		recorder := newRecorder()
		stat := readyContainerRegistryStepStatStub{
			size: v1alpha2.ImageStatusSize{
				Stored:   "10Gi",
				Unpacked: "12Gi",
			},
			dvcrImageName: "registry.example.com/image:tag",
			format:        "qcow2",
			cdrom:         true,
		}

		result, err := NewReadyContainerRegistryStep(pod, diskService, importer, stat, recorder, cb).Take(context.Background(), vi)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(*result).To(Equal(reconcile.Result{}))
		Expect(importer.calls).To(Equal(1))
		Expect(diskService.calls).To(Equal(1))
		Expect(recorder.EventCalls()).To(HaveLen(1))
		Expect(recorder.EventCalls()[0].Eventtype).To(Equal(corev1.EventTypeNormal))
		Expect(recorder.EventCalls()[0].Reason).To(Equal(v1alpha2.ReasonDataSourceSyncCompleted))
		Expect(recorder.EventCalls()[0].Message).To(Equal("The ObjectRef DataSource import has completed"))
		Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageReady))
		Expect(vi.Status.Size).To(Equal(stat.size))
		Expect(vi.Status.CDROM).To(BeTrue())
		Expect(vi.Status.Format).To(Equal("qcow2"))
		Expect(vi.Status.Progress).To(Equal("100%"))
		Expect(vi.Status.Target.RegistryURL).To(Equal("registry.example.com/image:tag"))
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionTrue))
		Expect(cb.Condition().Reason).To(Equal(vicondition.Ready.String()))
		Expect(cb.Condition().Message).To(BeEmpty())
	})
})
