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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type WaitForPVCStep struct {
	pvc *corev1.PersistentVolumeClaim
	cb  *conditions.ConditionBuilder
}

func NewWaitForPVCStep(
	pvc *corev1.PersistentVolumeClaim,
	cb *conditions.ConditionBuilder,
) *WaitForPVCStep {
	return &WaitForPVCStep{
		pvc: pvc,
		cb:  cb,
	}
}

func (s WaitForPVCStep) Take(_ context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	if s.pvc == nil {
		vi.Status.Phase = virtv2.ImageProvisioning
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Waiting for the underlying PersistentVolumeClaim to be created by controller.")

		return &reconcile.Result{}, nil
	}

	if s.pvc.Status.Phase == corev1.ClaimBound {
		return nil, nil
	}

	vi.Status.Phase = virtv2.ImageProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message(fmt.Sprintf("Waiting for the PVC %s to be Bound.", s.pvc.Name))

	return &reconcile.Result{}, nil
}
