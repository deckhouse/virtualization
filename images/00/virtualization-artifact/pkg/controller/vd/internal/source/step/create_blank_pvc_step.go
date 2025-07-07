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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	pvcspec "github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const createStep = "create"

type VolumeAndAccessModesGetter interface {
	GetVolumeAndAccessModes(ctx context.Context, sc *storev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error)
}

type CreateBlankPVCStep struct {
	pvc        *corev1.PersistentVolumeClaim
	modeGetter VolumeAndAccessModesGetter
	client     client.Client
	cb         *conditions.ConditionBuilder
}

func NewCreateBlankPVCStep(
	pvc *corev1.PersistentVolumeClaim,
	modeGetter VolumeAndAccessModesGetter,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *CreateBlankPVCStep {
	return &CreateBlankPVCStep{
		pvc:        pvc,
		modeGetter: modeGetter,
		client:     client,
		cb:         cb,
	}
}

func (s CreateBlankPVCStep) Take(ctx context.Context, vd *virtv2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc != nil {
		return nil, nil
	}

	if vd.Status.StorageClassName == "" {
		return nil, errors.New("storage class name is omitted in status but expected to be set")
	}

	if vd.Spec.PersistentVolumeClaim.Size == nil || vd.Spec.PersistentVolumeClaim.Size.IsZero() {
		return nil, errors.New("spec.persistentVolumeClaim.size should be set for blank virtual disk")
	}

	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.StorageClassName}, s.client, &storev1.StorageClass{})
	if err != nil {
		return nil, fmt.Errorf("get storage class: %w", err)
	}

	if sc == nil {
		return nil, fmt.Errorf("storage class %q not found", vd.Status.StorageClassName)
	}

	volumeMode, accessMode, err := s.modeGetter.GetVolumeAndAccessModes(ctx, sc)
	if err != nil {
		return nil, fmt.Errorf("get volume and access modes: %w", err)
	}

	key := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID).PersistentVolumeClaim()
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
			Finalizers: []string{
				virtv2.FinalizerVDProtection,
			},
			OwnerReferences: []metav1.OwnerReference{
				service.MakeOwnerReference(vd),
			},
		},
		Spec: ptr.Deref(
			pvcspec.CreateSpec(&sc.Name, *vd.Spec.PersistentVolumeClaim.Size, accessMode, volumeMode),
			corev1.PersistentVolumeClaimSpec{},
		),
	}

	log := logger.FromContext(ctx).With(logger.SlogStep(createStep)).With("pvc.name", pvc.Name)
	log.Debug("Create new PVC")

	err = s.client.Create(ctx, &pvc)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		if strings.Contains(err.Error(), "exceeded quota") {
			log.Debug("Quota exceeded during PVC creation")

			vd.Status.Phase = virtv2.DiskPending
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.QuotaExceeded).
				Message(fmt.Sprintf("Quota exceeded during the creation of underlying PersistentVolumeClaim %q.", pvc.Name))

			return &reconcile.Result{}, nil
		}

		return nil, fmt.Errorf("create pvc: %w", err)
	}

	vd.Status.Progress = "0%"
	vd.Status.Target.PersistentVolumeClaim = pvc.Name

	log.Debug("PVC has been created")

	return nil, nil
}
