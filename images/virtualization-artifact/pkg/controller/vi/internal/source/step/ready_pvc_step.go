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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ReadyPersistentVolumeClaimStepBounder interface {
	CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error)
}

type ReadyPersistentVolumeClaimStep struct {
	pvc      *corev1.PersistentVolumeClaim
	bounder  ReadyPersistentVolumeClaimStepBounder
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewReadyPersistentVolumeClaimStep(
	pvc *corev1.PersistentVolumeClaim,
	bounder ReadyPersistentVolumeClaimStepBounder,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *ReadyPersistentVolumeClaimStep {
	return &ReadyPersistentVolumeClaimStep{
		pvc:      pvc,
		bounder:  bounder,
		recorder: recorder,
		cb:       cb,
	}
}

func (s ReadyPersistentVolumeClaimStep) Take(ctx context.Context, vi *v1alpha2.VirtualImage) (*reconcile.Result, error) {
	log, _ := logger.GetDataSourceContext(ctx, "objectref")

	if s.pvc == nil {
		ready, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
		if ready.Status == metav1.ConditionTrue {
			log.Debug("PVC is lost", ".status.target.pvc", vi.Status.Target.PersistentVolumeClaim)

			vi.Status.Phase = v1alpha2.ImagePVCLost
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.PVCLost).
				Message(fmt.Sprintf("PersistentVolumeClaim %q not found.", vi.Status.Target.PersistentVolumeClaim))
			return &reconcile.Result{}, nil
		}

		return nil, nil
	}

	vi.Status.Target.PersistentVolumeClaim = s.pvc.Name

	switch s.pvc.Status.Phase {
	case corev1.ClaimLost:
		log.Warn("Image is Lost: underlying PVC is Lost")

		vi.Status.Phase = v1alpha2.ImagePVCLost
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.PVCLost).
			Message(fmt.Sprintf("PersistentVolume %q not found.", s.pvc.Spec.VolumeName))

		return &reconcile.Result{}, nil
	case corev1.ClaimBound:
		log.Debug("Image is Ready")

		err := s.cleanUpSupplements(ctx, vi)
		if err != nil {
			return nil, fmt.Errorf("clean up supplements: %w", err)
		}

		if vi.Status.Phase != v1alpha2.ImageReady {
			s.recorder.Event(
				vi,
				corev1.EventTypeNormal,
				v1alpha2.ReasonDataSourceSyncCompleted,
				"The ObjectRef DataSource import has completed",
			)
		}

		s.cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")

		vi.Status.Phase = v1alpha2.ImageReady
		vi.Status.Progress = "100%"

		res := s.pvc.Status.Capacity[corev1.ResourceStorage]

		intQ, ok := res.AsInt64()
		if !ok {
			return nil, errors.New("failed to convert quantity to int64")
		}

		vi.Status.Size = v1alpha2.ImageStatusSize{
			Stored:        res.String(),
			StoredBytes:   strconv.FormatInt(intQ, 10),
			Unpacked:      res.String(),
			UnpackedBytes: strconv.FormatInt(intQ, 10),
		}

		return &reconcile.Result{}, nil
	default:
		return nil, nil
	}
}

func (s ReadyPersistentVolumeClaimStep) cleanUpSupplements(ctx context.Context, vi *v1alpha2.VirtualImage) error {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	_, err := s.bounder.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return err
	}

	return nil
}
