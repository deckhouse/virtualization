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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type WaitForPodStepStat interface {
	GetProgress(ownerUID types.UID, pod *corev1.Pod, prevProgress string, opts ...service.GetProgressOption) string
	GetDVCRImageName(pod *corev1.Pod) string
	CheckPod(pod *corev1.Pod) error
}

type WaitForPodStep struct {
	pod  *corev1.Pod
	pvc  *corev1.PersistentVolumeClaim
	stat WaitForPodStepStat
	cb   *conditions.ConditionBuilder
}

func NewWaitForPodStep(
	pod *corev1.Pod,
	pvc *corev1.PersistentVolumeClaim,
	stat WaitForPodStepStat,
	cb *conditions.ConditionBuilder,
) *WaitForPodStep {
	return &WaitForPodStep{
		pod:  pod,
		pvc:  pvc,
		stat: stat,
		cb:   cb,
	}
}

func (s WaitForPodStep) Take(_ context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	if s.pod == nil {
		vi.Status.Phase = virtv2.ImageProvisioning
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Waiting for the importer pod to be created by controller.")

		return &reconcile.Result{}, nil
	}

	err := s.stat.CheckPod(s.pod)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
			if strings.Contains(err.Error(), "pod has unbound immediate PersistentVolumeClaims") {
				vi.Status.Phase = virtv2.ImageProvisioning
				s.cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.Provisioning).
					Message("Waiting for PersistentVolumeClaim to be Bound")

				return &reconcile.Result{Requeue: true}, nil
			}

			vi.Status.Phase = virtv2.ImageFailed
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.ProvisioningNotStarted).
				Message(service.CapitalizeFirstLetter(err.Error() + "."))
			return &reconcile.Result{}, nil
		case errors.Is(err, service.ErrProvisioningFailed):
			vi.Status.Phase = virtv2.ImageFailed
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.ProvisioningFailed).
				Message(service.CapitalizeFirstLetter(err.Error() + "."))
			return &reconcile.Result{}, nil
		default:
			vi.Status.Phase = virtv2.ImageFailed
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.ProvisioningFailed).
				Message(service.CapitalizeFirstLetter(err.Error() + "."))
			return &reconcile.Result{}, err
		}
	}

	if s.pod.Status.Phase != corev1.PodRunning {
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Preparing to start import to DVCR.")

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Target.RegistryURL = s.stat.GetDVCRImageName(s.pod)

		return &reconcile.Result{}, nil
	}

	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vicondition.Provisioning).
		Message("Import is in the process of provisioning to DVCR.")

	vi.Status.Phase = virtv2.ImageProvisioning
	vi.Status.Progress = s.stat.GetProgress(vi.GetUID(), s.pod, vi.Status.Progress)
	vi.Status.Target.RegistryURL = s.stat.GetDVCRImageName(s.pod)

	return &reconcile.Result{RequeueAfter: 2 * time.Second}, nil
}
