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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const readyStep = "ready"

type ReadyStepDiskService interface {
	GetCapacity(pvc *corev1.PersistentVolumeClaim) string
}

type ReadyStep struct {
	diskService ReadyStepDiskService
	pvc         *corev1.PersistentVolumeClaim
	cb          *conditions.ConditionBuilder
}

func NewReadyStep(
	diskService ReadyStepDiskService,
	pvc *corev1.PersistentVolumeClaim,
	cb *conditions.ConditionBuilder,
) *ReadyStep {
	return &ReadyStep{
		diskService: diskService,
		pvc:         pvc,
		cb:          cb,
	}
}

func (s ReadyStep) Take(ctx context.Context, vd *virtv2.VirtualDisk) (*reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogStep(readyStep))

	if s.pvc == nil {
		ready, _ := conditions.GetCondition(vdcondition.Ready, vd.Status.Conditions)
		if ready.Status == metav1.ConditionTrue {
			log.Debug("PVC is lost", ".status.target.pvc", vd.Status.Target.PersistentVolumeClaim)

			vd.Status.Phase = virtv2.DiskLost
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Lost).
				Message(fmt.Sprintf("PersistentVolumeClaim %q not found.", vd.Status.Target.PersistentVolumeClaim))
			return &reconcile.Result{}, nil
		}

		log.Debug("PVC not created yet")

		return nil, nil
	}

	vd.Status.Target.PersistentVolumeClaim = s.pvc.Name

	switch s.pvc.Status.Phase {
	case corev1.ClaimLost:
		vd.Status.Phase = virtv2.DiskLost
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Lost).
			Message(fmt.Sprintf("PersistentVolume %q not found.", s.pvc.Spec.VolumeName))

		log.Debug("PVC is Lost")

		return &reconcile.Result{}, nil
	case corev1.ClaimBound:
		vd.Status.Phase = virtv2.DiskReady
		s.cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")
		vd.Status.Progress = "100%"
		vd.Status.Capacity = s.diskService.GetCapacity(s.pvc)

		log.Debug("PVC is Bound")

		return &reconcile.Result{}, nil
	default:
		log.Debug("PVC not bound yet")

		return nil, nil
	}
}
