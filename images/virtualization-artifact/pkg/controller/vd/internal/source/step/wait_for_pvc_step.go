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
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type WaitForPVCStep struct {
	pvc    *corev1.PersistentVolumeClaim
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewWaitForPVCStep(
	pvc *corev1.PersistentVolumeClaim,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *WaitForPVCStep {
	return &WaitForPVCStep{
		pvc:    pvc,
		client: client,
		cb:     cb,
	}
}

func (s WaitForPVCStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if vd.Status.Progress == "" {
		vd.Status.Progress = "0%"
	}

	if s.pvc == nil {
		vd.Status.Phase = v1alpha2.DiskProvisioning
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("Waiting for the underlying PersistentVolumeClaim to be created.")
		return &reconcile.Result{}, nil
	}

	if s.pvc.Status.Phase == corev1.ClaimBound {
		return nil, nil
	}

	wffc, err := isStorageClassWFFC(ctx, s.client, ptr.Deref(s.pvc.Spec.StorageClassName, ""))
	if err != nil {
		return nil, fmt.Errorf("is wffc: %w", err)
	}

	if wffc {
		vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.WaitingForFirstConsumer).
			Message("Awaiting the creation and scheduling of the VirtualMachine with the attached VirtualDisk.")
		return &reconcile.Result{}, nil
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message(fmt.Sprintf("Waiting for the PersistentVolumeClaim %q to be Bound.", s.pvc.Name))
	return &reconcile.Result{}, nil
}

// isStorageClassWFFC reports whether the named storage class uses the
// WaitForFirstConsumer volume binding mode.
func isStorageClassWFFC(ctx context.Context, c client.Client, storageClassName string) (bool, error) {
	if storageClassName == "" {
		return false, nil
	}

	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: storageClassName}, c, &storagev1.StorageClass{})
	if err != nil {
		return false, fmt.Errorf("fetch storage class: %w", err)
	}

	return sc != nil && sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer, nil
}
