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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type createPodStepImporterStub struct {
	startErr         error
	startCalls       int
	podSettingsCalls int
	settings         *importer.Settings
	podSettings      *importer.PodSettings
}

func (s *createPodStepImporterStub) GetPodSettingsWithPVC(_ *metav1.OwnerReference, _ supplements.Generator, _, _ string) *importer.PodSettings {
	s.podSettingsCalls++
	if s.podSettings != nil {
		return s.podSettings
	}

	return &importer.PodSettings{}
}

func (s *createPodStepImporterStub) StartWithPodSetting(_ context.Context, settings *importer.Settings, _ supplements.Generator, _ *datasource.CABundle, podSettings *importer.PodSettings, _ ...service.Option) error {
	s.startCalls++
	s.settings = settings
	s.podSettings = podSettings
	return s.startErr
}

type createPodStepStatStub struct {
	dvcrImageName string
}

func (s createPodStepStatStub) GetSize(_ *corev1.Pod) v1alpha2.ImageStatusSize {
	return v1alpha2.ImageStatusSize{}
}
func (s createPodStepStatStub) GetDVCRImageName(_ *corev1.Pod) string { return s.dvcrImageName }
func (s createPodStepStatStub) GetFormat(_ *corev1.Pod) string        { return "" }
func (s createPodStepStatStub) GetCDROM(_ *corev1.Pod) bool           { return false }

var _ = Describe("CreatePodStep", func() {
	newCreatePodScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	newCreatePodVI := func() *v1alpha2.VirtualImage {
		return &v1alpha2.VirtualImage{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualImageKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vi",
				Namespace:         "default",
				UID:               types.UID("vi-uid"),
				CreationTimestamp: metav1.NewTime(time.Now()),
			},
			Spec: v1alpha2.VirtualImageSpec{
				DataSource: v1alpha2.VirtualImageDataSource{
					ObjectRef: &v1alpha2.VirtualImageObjectRef{Name: "snapshot"},
				},
			},
		}
	}

	newCreatePodObjects := func(volumeMode *corev1.PersistentVolumeMode) []client.Object {
		return []client.Object{
			&v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
				Spec:       v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "disk"},
			},
			&v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "disk", Namespace: "default"},
				Status:     v1alpha2.VirtualDiskStatus{Target: v1alpha2.DiskTarget{PersistentVolumeClaim: "disk-pvc"}},
			},
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "disk-pvc", Namespace: "default"},
				Spec:       corev1.PersistentVolumeClaimSpec{VolumeMode: volumeMode},
			},
		}
	}

	newCreatePodRecorder := func() *eventrecord.EventRecorderLoggerMock {
		var recorder *eventrecord.EventRecorderLoggerMock
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc: noopEvent,
			WithLoggingFunc: func(eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
				return recorder
			},
		}
		return recorder
	}

	newCreatePodStep := func(
		pod *corev1.Pod,
		objects []client.Object,
		importerStub *createPodStepImporterStub,
		recorder *eventrecord.EventRecorderLoggerMock,
		stat createPodStepStatStub,
		cb *conditions.ConditionBuilder,
		settings *dvcr.Settings,
	) *CreatePodStep {
		return NewCreatePodStep(
			pod,
			fake.NewClientBuilder().WithScheme(newCreatePodScheme()).WithObjects(objects...).Build(),
			settings,
			recorder,
			importerStub,
			stat,
			cb,
		)
	}

	It("skips when pod already exists", func() {
		vi := newCreatePodVI()
		cb := conditions.NewConditionBuilder(vicondition.ReadyType)
		importerStub := &createPodStepImporterStub{}

		result, err := newCreatePodStep(
			&corev1.Pod{},
			nil,
			importerStub,
			newCreatePodRecorder(),
			createPodStepStatStub{},
			cb,
			&dvcr.Settings{},
		).Take(context.Background(), vi)

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(importerStub.podSettingsCalls).To(BeZero())
		Expect(importerStub.startCalls).To(BeZero())
	})

	DescribeTable(
		"returns get errors before importer start",
		func(objects []client.Object, expectedErr error) {
			vi := newCreatePodVI()
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)
			importerStub := &createPodStepImporterStub{}

			result, err := newCreatePodStep(
				nil,
				objects,
				importerStub,
				newCreatePodRecorder(),
				createPodStepStatStub{},
				cb,
				&dvcr.Settings{},
			).Take(context.Background(), vi)

			Expect(err).To(MatchError(expectedErr))
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))
			Expect(importerStub.startCalls).To(BeZero())
		},
		Entry("when virtual disk snapshot is missing", []client.Object{}, apierrors.NewNotFound(v1alpha2.Resource("virtualdisksnapshots"), "snapshot")),
		Entry("when virtual disk is missing", []client.Object{
			&v1alpha2.VirtualDiskSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"}, Spec: v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "disk"}},
		}, apierrors.NewNotFound(v1alpha2.Resource("virtualdisks"), "disk")),
		Entry("when pvc is missing", []client.Object{
			&v1alpha2.VirtualDiskSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"}, Spec: v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "disk"}},
			&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "disk", Namespace: "default"}, Status: v1alpha2.VirtualDiskStatus{Target: v1alpha2.DiskTarget{PersistentVolumeClaim: "disk-pvc"}}},
		}, apierrors.NewNotFound(corev1.Resource("persistentvolumeclaims"), "disk-pvc")),
	)

	DescribeTable(
		"handles importer start errors",
		func(
			creationTimestamp metav1.Time,
			startErr error,
			expectedErr error,
			expectedMessage string,
			expectedEventCalls int,
			expectedRequeueAfter time.Duration,
		) {
			vi := newCreatePodVI()
			vi.CreationTimestamp = creationTimestamp
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)
			importerStub := &createPodStepImporterStub{startErr: startErr}
			recorder := newCreatePodRecorder()

			result, err := newCreatePodStep(
				nil,
				newCreatePodObjects(nil),
				importerStub,
				recorder,
				createPodStepStatStub{},
				cb,
				&dvcr.Settings{},
			).Take(context.Background(), vi)

			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.RequeueAfter).To(Equal(expectedRequeueAfter))
			} else {
				Expect(err).To(MatchError(expectedErr))
				Expect(result).To(BeNil())
			}

			Expect(importerStub.startCalls).To(Equal(1))
			Expect(vi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
			Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
			Expect(cb.Condition().Reason).To(Equal(vicondition.ProvisioningFailed.String()))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
			Expect(recorder.EventCalls()).To(HaveLen(expectedEventCalls))
		},
		Entry(
			"quota exceeded for a fresh object",
			metav1.NewTime(time.Now()),
			errors.New("exceeded quota: namespace quota"),
			nil,
			"Quota exceeded: exceeded quota: namespace quota; Please configure quotas or try recreating the resource later.",
			1,
			time.Duration(0),
		),
		Entry(
			"quota exceeded for an old object",
			metav1.NewTime(time.Now().Add(-31*time.Minute)),
			errors.New("exceeded quota: namespace quota"),
			nil,
			"Quota exceeded: exceeded quota: namespace quota; Retry in 1 minute.",
			1,
			time.Minute,
		),
		Entry(
			"unknown error",
			metav1.NewTime(time.Now()),
			errors.New("boom"),
			errors.New("boom"),
			"Unexpected error: boom",
			0,
			time.Duration(0),
		),
	)

	DescribeTable(
		"getEnvSettings chooses source type from pvc volume mode",
		func(volumeMode *corev1.PersistentVolumeMode, expectedSource string) {
			vi := newCreatePodVI()
			settings := &dvcr.Settings{RegistryURL: "registry.example.com", AuthSecret: "dvcr-secret", AuthSecretNamespace: "default"}
			step := newCreatePodStep(nil, nil, &createPodStepImporterStub{}, newCreatePodRecorder(), createPodStepStatStub{}, conditions.NewConditionBuilder(vicondition.ReadyType), settings)

			envSettings := step.getEnvSettings(vi, supplements.NewGenerator("vi", vi.Name, vi.Namespace, vi.UID), volumeMode)
			Expect(envSettings.Source).To(Equal(expectedSource))
			Expect(envSettings.DestinationAuthSecret).To(Equal("dvcr-secret"))
			Expect(envSettings.DestinationEndpoint).To(Equal("registry.example.com/vi/default/vi:vi-uid"))
		},
		Entry("filesystem by default", nil, importer.SourceFilesystem),
		Entry("block device for block volume mode", ptr.To(corev1.PersistentVolumeBlock), importer.SourceBlockDevice),
	)

	It("starts importer and updates image status", func() {
		vi := newCreatePodVI()
		cb := conditions.NewConditionBuilder(vicondition.ReadyType)
		importerStub := &createPodStepImporterStub{}
		stat := createPodStepStatStub{dvcrImageName: "registry.example.com/custom:tag"}

		result, err := newCreatePodStep(
			nil,
			newCreatePodObjects(ptr.To(corev1.PersistentVolumeBlock)),
			importerStub,
			newCreatePodRecorder(),
			stat,
			cb,
			&dvcr.Settings{RegistryURL: "registry.example.com", AuthSecret: "dvcr-secret", AuthSecretNamespace: "default"},
		).Take(context.Background(), vi)

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(importerStub.podSettingsCalls).To(Equal(1))
		Expect(importerStub.startCalls).To(Equal(1))
		Expect(importerStub.settings).ToNot(BeNil())
		Expect(importerStub.settings.Source).To(Equal(importer.SourceBlockDevice))
		Expect(importerStub.settings.DestinationAuthSecret).To(Equal("dvcr-secret"))
		Expect(importerStub.settings.DestinationEndpoint).To(Equal("registry.example.com/vi/default/vi:vi-uid"))
		Expect(vi.Status.Progress).To(Equal("0%"))
		Expect(vi.Status.Target.RegistryURL).To(Equal("registry.example.com/custom:tag"))
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionUnknown))
	})
})
