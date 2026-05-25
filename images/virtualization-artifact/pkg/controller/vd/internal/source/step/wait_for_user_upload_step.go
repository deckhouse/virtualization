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
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type WaitForUserUploadStepStatService interface {
	CheckPod(pod *corev1.Pod) error
	IsUploadStarted(ownerUID types.UID, pod *corev1.Pod) bool
	IsUploaderReady(pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress, tlsSecret *corev1.Secret) (bool, error)
}

type WaitForDVCRUploaderStepStatService interface {
	CheckPod(pod *corev1.Pod) error
	IsUploadStarted(ownerUID types.UID, pod *corev1.Pod) bool
	GetProgress(ownerUID types.UID, pod *corev1.Pod, prevProgress string, opts ...service.GetProgressOption) string
	GetDownloadSpeed(ownerUID types.UID, pod *corev1.Pod) *v1alpha2.StatusSpeed
}

type WaitForUserUploadStepUploaderService interface {
	GetExternalURL(ctx context.Context, ing *netv1.Ingress) string
	GetInClusterURL(ctx context.Context, svc *corev1.Service) string
}

type WaitForUserUploadStep struct {
	pod      *corev1.Pod
	svc      *corev1.Service
	ing      *netv1.Ingress
	stat     WaitForUserUploadStepStatService
	uploader WaitForUserUploadStepUploaderService
	client   client.Client
	cb       *conditions.ConditionBuilder
}

func NewWaitForUserUploadStep(
	pod *corev1.Pod,
	svc *corev1.Service,
	ing *netv1.Ingress,
	stat WaitForUserUploadStepStatService,
	uploader WaitForUserUploadStepUploaderService,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *WaitForUserUploadStep {
	return &WaitForUserUploadStep{
		pod:      pod,
		svc:      svc,
		ing:      ing,
		stat:     stat,
		uploader: uploader,
		client:   client,
		cb:       cb,
	}
}

func (s WaitForUserUploadStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pod == nil || podutil.IsPodComplete(s.pod) {
		return nil, nil
	}

	supgen := vdsupplements.NewGenerator(vd)
	uploadStarted := s.stat.IsUploadStarted(vd.GetUID(), s.pod) || hasUploadProgress(vd.Status.Progress)
	if uploadStarted {
		return nil, nil
	}

	if err := s.stat.CheckPod(s.pod); err != nil {
		return s.handlePodError(ctx, vd, err)
	}

	tlsSecret, err := supplements.GetTLSSecret(ctx, s.client, supgen.Generator)
	if err != nil {
		return nil, fmt.Errorf("fetch uploader tls secret: %w", err)
	}

	isUploaderReady, err := s.stat.IsUploaderReady(s.pod, s.svc, s.ing, tlsSecret)
	if err != nil {
		return nil, fmt.Errorf("check uploader readiness: %w", err)
	}

	if isUploaderReady {
		vd.Status.Phase = v1alpha2.DiskWaitForUserUpload
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.WaitForUserUpload).
			Message("Waiting for the user upload.")
		vd.Status.ImageUploadURLs = &v1alpha2.ImageUploadURLs{
			External:  s.uploader.GetExternalURL(ctx, s.ing),
			InCluster: s.uploader.GetInClusterURL(ctx, s.svc),
		}
	} else {
		vd.Status.Phase = v1alpha2.DiskProvisioning
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message(fmt.Sprintf("Waiting for the uploader %q to be ready to process the user's upload.", s.pod.Name))
	}

	return &reconcile.Result{RequeueAfter: time.Second}, nil
}

type WaitForDVCRUploaderStep struct {
	pod  *corev1.Pod
	stat WaitForDVCRUploaderStepStatService
	cb   *conditions.ConditionBuilder
}

func NewWaitForDVCRUploaderStep(
	pod *corev1.Pod,
	stat WaitForDVCRUploaderStepStatService,
	cb *conditions.ConditionBuilder,
) *WaitForDVCRUploaderStep {
	return &WaitForDVCRUploaderStep{
		pod:  pod,
		stat: stat,
		cb:   cb,
	}
}

func (s WaitForDVCRUploaderStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pod == nil || podutil.IsPodComplete(s.pod) {
		return nil, nil
	}

	uploadStarted := s.stat.IsUploadStarted(vd.GetUID(), s.pod) || hasUploadProgress(vd.Status.Progress)
	if !uploadStarted {
		return nil, nil
	}

	if err := s.stat.CheckPod(s.pod); err != nil {
		return handleUploaderPodError(vd, err, s.cb, true)
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("Import is in the process of provisioning to DVCR.")

	vd.Status.Progress = s.stat.GetProgress(vd.GetUID(), s.pod, vd.Status.Progress, service.NewScaleOption(0, 50))
	vd.Status.DownloadSpeed = s.stat.GetDownloadSpeed(vd.GetUID(), s.pod)

	return &reconcile.Result{RequeueAfter: time.Second}, nil
}

func (s WaitForUserUploadStep) handlePodError(_ context.Context, vd *v1alpha2.VirtualDisk, podErr error) (*reconcile.Result, error) {
	return handleUploaderPodError(vd, podErr, s.cb, false)
}

func handleUploaderPodError(vd *v1alpha2.VirtualDisk, podErr error, cb *conditions.ConditionBuilder, uploadStarted bool) (*reconcile.Result, error) {
	switch {
	case errors.Is(podErr, service.ErrNotInitialized), errors.Is(podErr, service.ErrNotScheduled):
		if uploadStarted {
			vd.Status.Phase = v1alpha2.DiskProvisioning
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Provisioning).
				Message("Import is in the process of provisioning to DVCR.")
			return &reconcile.Result{RequeueAfter: time.Second}, nil
		}

		vd.Status.Phase = v1alpha2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message(service.CapitalizeFirstLetter(podErr.Error()) + ".")
		return &reconcile.Result{}, nil
	case errors.Is(podErr, service.ErrProvisioningFailed):
		vd.Status.Phase = v1alpha2.DiskFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(podErr.Error()) + ".")
		return &reconcile.Result{}, nil
	default:
		vd.Status.Phase = v1alpha2.DiskFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(fmt.Errorf("unexpected error: %w", podErr).Error()) + ".")
		return nil, podErr
	}
}

func hasUploadProgress(progress string) bool {
	switch progress {
	case "", "0", "0%", "0.0%", "0.00%":
		return false
	default:
		return true
	}
}
