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
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ReadyStepDiskService interface {
	GetCapacity(pvc *corev1.PersistentVolumeClaim) string
	Protect(ctx context.Context, owner client.Object, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error
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
	if s.pvc == nil {
		if vd.Status.Target.PersistentVolumeClaim != "" {
			vd.Status.Phase = virtv2.DiskLost
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Lost).
				Message(fmt.Sprintf("PersistentVolumeClaim %q not found.", vd.Status.Target.PersistentVolumeClaim))
			return &reconcile.Result{}, nil
		}

		return nil, nil
	}

	switch s.pvc.Status.Phase {
	case corev1.ClaimLost:
		vd.Status.Phase = virtv2.DiskLost
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Lost).
			Message(fmt.Sprintf("PersistentVolume %q not found.", s.pvc.Spec.VolumeName))

		return &reconcile.Result{}, nil
	case corev1.ClaimBound:
		vd.Status.Phase = virtv2.DiskReady
		s.cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")
		vd.Status.Progress = "100%"
		vd.Status.Capacity = s.diskService.GetCapacity(s.pvc)
		vd.Status.Target.PersistentVolumeClaim = s.pvc.Name

		err := s.diskService.Protect(ctx, vd, nil, s.pvc)
		if err != nil {
			return nil, fmt.Errorf("protect: %w", err)
		}

		return &reconcile.Result{}, nil
	default:
		return nil, nil
	}
}
