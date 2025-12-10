/*
Copyright 2025 Flant JSC

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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type CreatePodStepImporter interface {
	GetPodSettingsWithPVC(_ *metav1.OwnerReference, _ *supplements.Generator, _, _ string) *importer.PodSettings
	StartWithPodSetting(_ context.Context, _ *importer.Settings, _ *supplements.Generator, _ *datasource.CABundle, _ *importer.PodSettings) error
}

type CreatePodStepStat interface {
	GetSize(pod *corev1.Pod) virtv2.ImageStatusSize
	GetDVCRImageName(pod *corev1.Pod) string
	GetFormat(pod *corev1.Pod) string
	GetCDROM(pod *corev1.Pod) bool
}

type CreatePodStep struct {
	pod          *corev1.Pod
	dvcrSettings *dvcr.Settings
	client       client.Client
	recorder     eventrecord.EventRecorderLogger
	importer     CreatePodStepImporter
	stat         CreatePodStepStat
	cb           *conditions.ConditionBuilder
}

func NewCreatePodStep(
	pod *corev1.Pod,
	client client.Client,
	dvcrSettings *dvcr.Settings,
	recorder eventrecord.EventRecorderLogger,
	importer CreatePodStepImporter,
	stat CreatePodStepStat,
	cb *conditions.ConditionBuilder,
) *CreatePodStep {
	return &CreatePodStep{
		pod:          pod,
		client:       client,
		dvcrSettings: dvcrSettings,
		recorder:     recorder,
		importer:     importer,
		stat:         stat,
		cb:           cb,
	}
}

func (s CreatePodStep) Take(ctx context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	if s.pod != nil {
		return nil, nil
	}

	ownerRef := metav1.NewControllerRef(vi, vi.GroupVersionKind())
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pvcKey := supgen.PersistentVolumeClaim()
	podSettings := s.importer.GetPodSettingsWithPVC(ownerRef, supgen, pvcKey.Name, pvcKey.Namespace)

	vds := &virtv2.VirtualDiskSnapshot{}
	err := s.client.Get(ctx, types.NamespacedName{Name: vi.Spec.DataSource.ObjectRef.Name, Namespace: vi.Namespace}, vds)
	if err != nil {
		return &reconcile.Result{}, err
	}

	vd := &virtv2.VirtualDisk{}
	err = s.client.Get(ctx, types.NamespacedName{Name: vds.Spec.VirtualDiskName, Namespace: vds.Namespace}, vd)
	if err != nil {
		return &reconcile.Result{}, err
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err = s.client.Get(ctx, types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}, pvc)
	if err != nil {
		return &reconcile.Result{}, err
	}

	envSettings := s.getEnvSettings(vi, supgen, pvc.Spec.VolumeMode)
	err = s.importer.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForVMI(vi.GetNamespace(), vi.Spec.DataSource), podSettings)
	switch {
	case err == nil:
		// OK.
	case common.ErrQuotaExceeded(err):
		s.recorder.Event(vi, corev1.EventTypeWarning, virtv2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
		return setQuotaExceededPhaseCondition(s.cb, &vi.Status.Phase, err, vi.CreationTimestamp), nil
	default:
		setPhaseConditionToFailed(s.cb, &vi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
		return nil, err
	}

	log, _ := logger.GetDataSourceContext(ctx, "objectref")
	log.Debug("The importer Pod has just been created.")

	vi.Status.Progress = "0%"
	vi.Status.Target.RegistryURL = s.stat.GetDVCRImageName(s.pod)

	return nil, nil
}

func (s CreatePodStep) getEnvSettings(vi *virtv2.VirtualImage, sup *supplements.Generator, volumeMode *corev1.PersistentVolumeMode) *importer.Settings {
	var settings importer.Settings

	if volumeMode != nil && *volumeMode == corev1.PersistentVolumeBlock {
		importer.ApplyBlockDeviceSourceSettings(&settings)
	} else {
		importer.ApplyFilesystemSourceSettings(&settings)
	}

	importer.ApplyDVCRDestinationSettings(
		&settings,
		s.dvcrSettings,
		sup,
		s.dvcrSettings.RegistryImageForVI(vi),
	)

	return &settings
}

const retryPeriod = 1

func setQuotaExceededPhaseCondition(cb *conditions.ConditionBuilder, phase *virtv2.ImagePhase, err error, creationTimestamp metav1.Time) *reconcile.Result {
	*phase = virtv2.ImageFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vicondition.ProvisioningFailed)

	if creationTimestamp.Add(30 * time.Minute).After(time.Now()) {
		cb.Message(fmt.Sprintf("Quota exceeded: %s; Please configure quotas or try recreating the resource later.", err))
		return &reconcile.Result{}
	}

	cb.Message(fmt.Sprintf("Quota exceeded: %s; Retry in %d minute.", err, retryPeriod))
	return &reconcile.Result{RequeueAfter: retryPeriod * time.Minute}
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *virtv2.ImagePhase, err error) {
	*phase = virtv2.ImageFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vicondition.ProvisioningFailed).
		Message(service.CapitalizeFirstLetter(err.Error()))
}
