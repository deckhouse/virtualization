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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ReadyContainerRegistryStepDiskService interface {
	CleanUpSupplements(ctx context.Context, sup *supplements.Generator) (bool, error)
}

type ReadyContainerRegistryStepImporter interface {
	CleanUpSupplements(ctx context.Context, sup *supplements.Generator) (bool, error)
}

type ReadyContainerRegistryStepStat interface {
	GetSize(pod *corev1.Pod) virtv2.ImageStatusSize
	GetDVCRImageName(pod *corev1.Pod) string
	GetFormat(pod *corev1.Pod) string
	CheckPod(pod *corev1.Pod) error
	GetCDROM(pod *corev1.Pod) bool
}

type ReadyContainerRegistryStep struct {
	pod      *corev1.Pod
	importer ReadyContainerRegistryStepImporter
	stat     ReadyContainerRegistryStepStat
	disk     ReadyContainerRegistryStepDiskService
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewReadyContainerRegistryStep(
	pod *corev1.Pod,
	importer ReadyContainerRegistryStepImporter,
	disk ReadyContainerRegistryStepDiskService,
	stat ReadyContainerRegistryStepStat,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *ReadyContainerRegistryStep {
	return &ReadyContainerRegistryStep{
		pod:      pod,
		importer: importer,
		disk:     disk,
		stat:     stat,
		recorder: recorder,
		cb:       cb,
	}
}

func (s ReadyContainerRegistryStep) Take(ctx context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	log, _ := logger.GetDataSourceContext(ctx, "objectref")

	ready, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	if ready.Status == metav1.ConditionTrue {
		log.Debug("Image is Ready")

		vi.Status.Phase = virtv2.ImageReady
		s.cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")

		return &reconcile.Result{}, nil
	}

	if !podutil.IsPodComplete(s.pod) {
		return nil, nil
	}

	err := s.stat.CheckPod(s.pod)
	if err != nil {
		vi.Status.Phase = virtv2.ImageFailed

		switch {
		case errors.Is(err, service.ErrProvisioningFailed):
			log.Debug("Provisioning is failed")

			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.ProvisioningFailed).
				Message(service.CapitalizeFirstLetter(err.Error() + "."))
			return &reconcile.Result{}, nil
		default:
			return nil, err
		}
	}

	err = s.cleanUpSupplements(ctx, vi)
	if err != nil {
		return nil, fmt.Errorf("clean up supplements: %w", err)
	}

	log.Info("Image is Ready now")

	s.recorder.Event(
		vi,
		corev1.EventTypeNormal,
		virtv2.ReasonDataSourceSyncCompleted,
		"The ObjectRef DataSource import has completed",
	)

	s.cb.
		Status(metav1.ConditionTrue).
		Reason(vicondition.Ready).
		Message("")

	vi.Status.Phase = virtv2.ImageReady
	vi.Status.Size = s.stat.GetSize(s.pod)
	vi.Status.CDROM = s.stat.GetCDROM(s.pod)
	vi.Status.Format = s.stat.GetFormat(s.pod)
	vi.Status.Progress = "100%"
	vi.Status.Target.RegistryURL = s.stat.GetDVCRImageName(s.pod)

	return &reconcile.Result{}, nil
}

func (s ReadyContainerRegistryStep) cleanUpSupplements(ctx context.Context, vi *virtv2.VirtualImage) error {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	_, err := s.importer.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return err
	}

	_, err = s.disk.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return err
	}

	return nil
}
