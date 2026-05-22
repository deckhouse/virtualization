/*
Copyright 2026 Flant JSC

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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type WaitForDVCRImporterStepStatService interface {
	CheckPod(pod *corev1.Pod) error
	GetProgress(ownerUID types.UID, pod *corev1.Pod, prevProgress string, opts ...service.GetProgressOption) string
	GetDownloadSpeed(ownerUID types.UID, pod *corev1.Pod) *v1alpha2.StatusSpeed
}

type WaitForDVCRImporterStepImporterService interface {
	Protect(ctx context.Context, pod *corev1.Pod, sup supplements.Generator) error
}

// WaitForDVCRImporterStep tracks an importer Pod that downloads source data into
// DVCR. It is a no-op while the Pod is missing or has already completed, in
// which case downstream steps take over.
type WaitForDVCRImporterStep struct {
	pod      *corev1.Pod
	stat     WaitForDVCRImporterStepStatService
	importer WaitForDVCRImporterStepImporterService
	client   client.Client
	cb       *conditions.ConditionBuilder
}

func NewWaitForDVCRImporterStep(
	pod *corev1.Pod,
	stat WaitForDVCRImporterStepStatService,
	importer WaitForDVCRImporterStepImporterService,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *WaitForDVCRImporterStep {
	return &WaitForDVCRImporterStep{
		pod:      pod,
		stat:     stat,
		importer: importer,
		client:   client,
		cb:       cb,
	}
}

func (s WaitForDVCRImporterStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pod == nil || podutil.IsPodComplete(s.pod) {
		return nil, nil
	}

	if err := s.stat.CheckPod(s.pod); err != nil {
		return s.handlePodError(ctx, vd, err)
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("Import is in the process of provisioning to DVCR.")

	vd.Status.Progress = s.stat.GetProgress(vd.GetUID(), s.pod, vd.Status.Progress, service.NewScaleOption(0, 50))
	vd.Status.DownloadSpeed = s.stat.GetDownloadSpeed(vd.GetUID(), s.pod)

	supgen := vdsupplements.NewGenerator(vd)
	if err := s.importer.Protect(ctx, s.pod, supgen); err != nil {
		return nil, fmt.Errorf("protect importer pod: %w", err)
	}

	return &reconcile.Result{RequeueAfter: 2 * time.Second}, nil
}

func (s WaitForDVCRImporterStep) handlePodError(ctx context.Context, vd *v1alpha2.VirtualDisk, podErr error) (*reconcile.Result, error) {
	switch {
	case errors.Is(podErr, service.ErrNotInitialized):
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningNotStarted).
			Message(service.CapitalizeFirstLetter(podErr.Error()) + ".")
		return &reconcile.Result{}, nil
	case errors.Is(podErr, service.ErrNotScheduled):
		vd.Status.Phase = v1alpha2.DiskPending

		nodePlacement, err := GetNodePlacement(ctx, s.client, vd)
		if err != nil {
			return nil, fmt.Errorf("get node placement: %w", err)
		}

		isChanged, err := provisioner.IsNodePlacementChanged(nodePlacement, s.pod)
		if err != nil {
			return nil, fmt.Errorf("check node placement: %w", err)
		}

		if isChanged {
			if err := s.client.Delete(ctx, s.pod); err != nil {
				return nil, fmt.Errorf("recreate importer pod: %w", err)
			}

			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ProvisioningNotStarted).
				Message("Provisioner recreation due to a changes in the virtual machine tolerations.")
		} else {
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ProvisioningNotStarted).
				Message(service.CapitalizeFirstLetter(podErr.Error()) + ".")
		}

		return &reconcile.Result{}, nil
	case errors.Is(podErr, service.ErrProvisioningFailed):
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(podErr.Error()) + ".")
		return &reconcile.Result{}, nil
	default:
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(fmt.Errorf("unexpected error: %w", podErr).Error()) + ".")
		return nil, podErr
	}
}
