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
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type WaitForDVStepDiskService interface {
	GetProgress(dv *cdiv1.DataVolume, prevProgress string, opts ...service.GetProgressOption) string
}

type WaitForDVStep struct {
	pvc    *corev1.PersistentVolumeClaim
	dv     *cdiv1.DataVolume
	disk   WaitForDVStepDiskService
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewWaitForDVStep(
	pvc *corev1.PersistentVolumeClaim,
	dv *cdiv1.DataVolume,
	disk WaitForDVStepDiskService,
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

func (s WaitForDVStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.dv == nil {
		vd.Status.Phase = v1alpha2.DiskProvisioning
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("Waiting for the VirtualDisk importer to be created.")
		return &reconcile.Result{}, nil
	}

	vd.Status.Progress = s.disk.GetProgress(s.dv, vd.Status.Progress, service.NewScaleOption(0, 100))
	vdsupplements.SetPVCName(vd, s.dv.Status.ClaimName)

	set, err := s.setForFirstConsumerIsAwaited(ctx, vd)
	if err != nil {
		return nil, fmt.Errorf("set for first consumer is awaited: %w", err)
	}
	ok := s.checkQoutaNotExceededCondition(vd, set)
	if !ok {
		return &reconcile.Result{}, nil
	}
	if set {
		return &reconcile.Result{}, nil
	}

	ok = s.checkRunningCondition(vd)
	if !ok {
		return &reconcile.Result{}, nil
	}

	ok, err = s.checkImporterPrimePod(ctx, vd)
	if err != nil {
		return nil, fmt.Errorf("check importer prime pod: %w", err)
	}
	if !ok {
		return &reconcile.Result{}, nil
	}

	set = s.setForProvisioning(vd)
	if set {
		return &reconcile.Result{}, nil
	}

	return nil, nil
}

func (s WaitForDVStep) setForProvisioning(vd *v1alpha2.VirtualDisk) (set bool) {
	if s.dv.Status.Phase != cdiv1.Succeeded {
		vd.Status.Phase = v1alpha2.DiskProvisioning
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("Import is in the process of provisioning to the PersistentVolumeClaim.")
		return true
	}

	return false
}

func (s WaitForDVStep) setForFirstConsumerIsAwaited(ctx context.Context, vd *v1alpha2.VirtualDisk) (set bool, err error) {
	if vd.Status.StorageClassName == "" {
		return false, fmt.Errorf("StorageClassName is empty, please report a bug")
	}

	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.StorageClassName}, s.client, &storagev1.StorageClass{})
	if err != nil {
		return false, fmt.Errorf("get sc: %w", err)
	}

	isWFFC := sc != nil && sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
	dvRunningCond, _ := conditions.GetDataVolumeCondition(conditions.DVRunningConditionType, s.dv.Status.Conditions)
	dvRunningReasonEmptyOrPending := dvRunningCond.Reason == "" || dvRunningCond.Reason == conditions.DVRunningConditionPendingReason
	if isWFFC && (s.dv.Status.Phase == cdiv1.PendingPopulation || s.dv.Status.Phase == cdiv1.WaitForFirstConsumer) && dvRunningReasonEmptyOrPending {
		vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.WaitingForFirstConsumer).
			Message("The provisioning has been suspended: a created and scheduled virtual machine is awaited.")
		return true, nil
	}

	return false, nil
}

func (s WaitForDVStep) checkQoutaNotExceededCondition(vd *v1alpha2.VirtualDisk, inwffc bool) (ok bool) {
	dvQuotaNotExceededCondition, _ := conditions.GetDataVolumeCondition(conditions.DVQoutaNotExceededConditionType, s.dv.Status.Conditions)
	if dvQuotaNotExceededCondition.Status == corev1.ConditionFalse {
		vd.Status.Phase = v1alpha2.DiskPending
		if inwffc {
			vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
		}
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.QuotaExceeded).
			Message(dvQuotaNotExceededCondition.Message)
		return false
	}

	return true
}

func (s WaitForDVStep) checkRunningCondition(vd *v1alpha2.VirtualDisk) (ok bool) {
	dvRunningCondition, _ := conditions.GetDataVolumeCondition(conditions.DVRunningConditionType, s.dv.Status.Conditions)
	switch {
	case dvRunningCondition.Reason == conditions.DVImagePullFailedReason:
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ImagePullFailed).
			Message(dvRunningCondition.Message)
		return false
	case strings.Contains(dvRunningCondition.Reason, "Error"):
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(dvRunningCondition.Message)
		return false
	default:
		return true
	}
}

func (s WaitForDVStep) checkImporterPrimePod(ctx context.Context, vd *v1alpha2.VirtualDisk) (ok bool, err error) {
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
			vd.Status.Phase = v1alpha2.DiskPending
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ImagePullFailed).
				Message(fmt.Sprintf("The PVC importer is not initialized; %s error %s: %s", cdiImporterPrimeKey.String(), podInitializedCond.Reason, podInitializedCond.Message))
			return false, nil
		}

		podScheduledCond, _ := conditions.GetPodCondition(corev1.PodScheduled, cdiImporterPrime.Status.Conditions)
		if podScheduledCond.Status == corev1.ConditionFalse && strings.Contains(podScheduledCond.Reason, "Error") {
			vd.Status.Phase = v1alpha2.DiskPending
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ImagePullFailed).
				Message(fmt.Sprintf("The PVC importer is not scheduled; %s error %s: %s", cdiImporterPrimeKey.String(), podScheduledCond.Reason, podScheduledCond.Message))
			return false, nil
		}
	}

	return true, nil
}
