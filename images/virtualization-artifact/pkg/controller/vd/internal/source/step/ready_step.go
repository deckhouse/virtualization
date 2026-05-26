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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ReadyStepDiskService interface {
	GetCapacity(pvc *corev1.PersistentVolumeClaim) string
	CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error)
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

func (s ReadyStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc == nil {
		if vd.Status.Progress == "100%" {
			vd.Status.Phase = v1alpha2.DiskLost
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Lost).
				Message(fmt.Sprintf("The PersistentVolumeClaim %q not found.", vd.Status.Target.PersistentVolumeClaim))
			return &reconcile.Result{}, nil
		}

		return nil, nil
	}

	vdsupplements.SetPVCName(vd, s.pvc.Name)
	// AnnPVCImportPhase is set on PVCs that go through the pvc-importer pipeline
	// (HTTP/Registry/Upload/ObjectRef CVI+VI/Clone). Blank and VDSnapshot data
	// sources never set it, so an empty phase means "no in-cluster import is
	// running" and we may fall through to the PVC.Status.Phase check below.
	// When it is set, we wait until the pvc-importer pod has succeeded before
	// declaring the disk Ready.
	if phase := s.pvc.GetAnnotations()[annotations.AnnPVCImportPhase]; phase != "" && phase != string(corev1.PodSucceeded) {
		return nil, nil
	}

	switch s.pvc.Status.Phase {
	case corev1.ClaimLost:
		if s.pvc.GetAnnotations()[annotations.AnnDataExportRequest] == "true" {
			vd.Status.Phase = v1alpha2.DiskExporting
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Exporting).
				Message("The VirtualDisk is being exported.")
		} else {
			vd.Status.Phase = v1alpha2.DiskLost
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Lost).
				Message(fmt.Sprintf("The PersistentVolume %q not found.", s.pvc.Spec.VolumeName))
		}

		return &reconcile.Result{}, nil
	case corev1.ClaimBound:
		vd.Status.Phase = v1alpha2.DiskReady
		s.cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")
		vd.Status.Progress = "100%"
		vd.Status.Capacity = s.diskService.GetCapacity(s.pvc)

		if object.ShouldCleanupSubResources(vd) {
			supgen := vdsupplements.NewGenerator(vd)
			if _, err := s.diskService.CleanUpSupplements(ctx, supgen); err != nil {
				return nil, fmt.Errorf("clean up supplements: %w", err)
			}
		}

		return &reconcile.Result{}, nil
	default:
		return nil, nil
	}
}
