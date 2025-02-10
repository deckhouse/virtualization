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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ReadyPersistentVolumeClaimStep struct {
	pvc      *corev1.PersistentVolumeClaim
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewReadyPersistentVolumeClaimStep(
	pvc *corev1.PersistentVolumeClaim,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *ReadyPersistentVolumeClaimStep {
	return &ReadyPersistentVolumeClaimStep{
		pvc:      pvc,
		recorder: recorder,
		cb:       cb,
	}
}

func (s ReadyPersistentVolumeClaimStep) Take(ctx context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	log, _ := logger.GetDataSourceContext(ctx, "objectref")

	if s.pvc == nil {
		if vi.Status.Target.PersistentVolumeClaim != "" {
			log.Warn("Image is Lost: underlying PVC not found")

			vi.Status.Phase = virtv2.ImageLost
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.Lost).
				Message(fmt.Sprintf("PersistentVolumeClaim %q not found.", vi.Status.Target.PersistentVolumeClaim))
			return &reconcile.Result{}, nil
		}

		return nil, nil
	}

	switch s.pvc.Status.Phase {
	case corev1.ClaimLost:
		log.Warn("Image is Lost: underlying PVC is Lost")

		vi.Status.Phase = virtv2.ImageLost
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Lost).
			Message(fmt.Sprintf("PersistentVolume %q not found.", s.pvc.Spec.VolumeName))

		return &reconcile.Result{}, nil
	case corev1.ClaimBound:
		log.Debug("Image is Ready")

		if vi.Status.Phase != virtv2.ImageReady {
			s.recorder.Event(
				vi,
				corev1.EventTypeNormal,
				virtv2.ReasonDataSourceSyncCompleted,
				"The ObjectRef DataSource import has completed",
			)
		}

		s.cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")

		vi.Status.Phase = virtv2.ImageReady
		vi.Status.Progress = "100%"

		var res resource.Quantity
		res = s.pvc.Status.Capacity[corev1.ResourceStorage]

		intQ, ok := res.AsInt64()
		if !ok {
			return nil, errors.New("failed to convert quantity to int64")
		}

		vi.Status.Size = virtv2.ImageStatusSize{
			Stored:        res.String(),
			StoredBytes:   strconv.FormatInt(intQ, 10),
			Unpacked:      res.String(),
			UnpackedBytes: strconv.FormatInt(intQ, 10),
		}

		vi.Status.Target.PersistentVolumeClaim = s.pvc.Name

		return &reconcile.Result{}, nil
	default:
		return nil, nil
	}
}
