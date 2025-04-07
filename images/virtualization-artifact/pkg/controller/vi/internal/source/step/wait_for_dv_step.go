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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type WaitForDVStepDisk interface {
	GetProgress(dv *cdiv1.DataVolume, prevProgress string, opts ...service.GetProgressOption) string
}

type WaitForDVStep struct {
	pvc    *corev1.PersistentVolumeClaim
	dv     *cdiv1.DataVolume
	disk   WaitForDVStepDisk
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewWaitForDVStep(
	pvc *corev1.PersistentVolumeClaim,
	dv *cdiv1.DataVolume,
	disk WaitForDVStepDisk,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *WaitForDVStep {
	return &WaitForDVStep{
		pvc:    pvc,
		dv:     dv,
		disk:   disk,
		client: client,
		cb:     cb,
	}
}

func (s WaitForDVStep) Take(ctx context.Context, vi *virtv2.VirtualImage) (*reconcile.Result, error) {
	if s.dv == nil {
		vi.Status.Phase = virtv2.ImageProvisioning
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Waiting for the VirtualDisk importer to be created.")
		return &reconcile.Result{}, nil
	}

	vi.Status.Progress = s.disk.GetProgress(s.dv, vi.Status.Progress, service.NewScaleOption(0, 100))

	ok := s.checkQoutaNotExceededCondition(vi)
	if !ok {
		return &reconcile.Result{}, nil
	}

	ok = s.checkRunningCondition(vi)
	if !ok {
		return &reconcile.Result{}, nil
	}

	ok, err := s.checkImporterPrimePod(ctx, vi)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &reconcile.Result{}, nil
	}

	return nil, nil
}

func (s WaitForDVStep) checkQoutaNotExceededCondition(vi *virtv2.VirtualImage) (ok bool) {
	dvQuotaNotExceededCondition, ok := conditions.GetDataVolumeCondition(conditions.DVQoutaNotExceededConditionType, s.dv.Status.Conditions)
	if ok && dvQuotaNotExceededCondition.Status == corev1.ConditionFalse {
		vi.Status.Phase = virtv2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.QuotaExceeded).
			Message(dvQuotaNotExceededCondition.Message)
		return false
	}

	return true
}

func (s WaitForDVStep) checkRunningCondition(vi *virtv2.VirtualImage) (ok bool) {
	dvRunningCondition, ok := conditions.GetDataVolumeCondition(conditions.DVRunningConditionType, s.dv.Status.Conditions)
	switch {
	case dvRunningCondition.Reason == conditions.DVImagePullFailedReason:
		vi.Status.Phase = virtv2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ImagePullFailed).
			Message(dvRunningCondition.Message)
		return false
	case strings.Contains(dvRunningCondition.Reason, "Error"):
		vi.Status.Phase = virtv2.ImagePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message(dvRunningCondition.Message)
		return false
	default:
		return true
	}
}

func (s WaitForDVStep) checkImporterPrimePod(ctx context.Context, vi *virtv2.VirtualImage) (ok bool, err error) {
	if s.pvc == nil {
		return true, nil
	}

	cdiImporterPrimeKey := types.NamespacedName{
		Namespace: s.pvc.Namespace,
		Name:      dvutil.GetImporterPrimeName(s.pvc.UID),
	}

	cdiImporterPrime, err := object.FetchObject(ctx, cdiImporterPrimeKey, s.client, &corev1.Pod{})
	if err != nil {
		return false, fmt.Errorf("fetch importer prime pod: %w", err)
	}

	if cdiImporterPrime != nil {
		podInitializedCond, _ := conditions.GetPodCondition(corev1.PodInitialized, cdiImporterPrime.Status.Conditions)
		if podInitializedCond.Status == corev1.ConditionFalse && strings.Contains(podInitializedCond.Reason, "Error") {
			vi.Status.Phase = virtv2.ImagePending
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.ImagePullFailed).
				Message(fmt.Sprintf("The PVC importer is not initialized; %s error %s: %s", cdiImporterPrimeKey.String(), podInitializedCond.Reason, podInitializedCond.Message))
			return false, nil
		}

		podScheduledCond, _ := conditions.GetPodCondition(corev1.PodScheduled, cdiImporterPrime.Status.Conditions)
		if podScheduledCond.Status == corev1.ConditionFalse && strings.Contains(podScheduledCond.Reason, "Error") {
			vi.Status.Phase = virtv2.ImagePending
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.ImagePullFailed).
				Message(fmt.Sprintf("The PVC importer is not scheduled; %s error %s: %s", cdiImporterPrimeKey.String(), podScheduledCond.Reason, podScheduledCond.Message))
			return false, nil
		}
	}

	return true, nil
}
