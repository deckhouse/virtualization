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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type CreateBounderPodStepBounder interface {
	Start(ctx context.Context, ownerRef *metav1.OwnerReference, sup supplements.Generator, opts ...service.Option) error
}

type CreateBounderPodStep struct {
	pvc      *corev1.PersistentVolumeClaim
	bounder  CreateBounderPodStepBounder
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewCreateBounderPodStep(
	pvc *corev1.PersistentVolumeClaim,
	bounder CreateBounderPodStepBounder,
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *CreateBounderPodStep {
	return &CreateBounderPodStep{
		pvc:      pvc,
		bounder:  bounder,
		client:   client,
		recorder: recorder,
		cb:       cb,
	}
}

func (s CreateBounderPodStep) Take(ctx context.Context, vi *v1alpha2.VirtualImage) (*reconcile.Result, error) {
	if s.pvc == nil {
		return nil, nil
	}

	wffc, err := s.isWFFC(ctx)
	if err != nil {
		return nil, err
	}

	if !wffc {
		return nil, nil
	}

	ownerRef := metav1.NewControllerRef(vi, vi.GroupVersionKind())

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	err = s.bounder.Start(ctx, ownerRef, supgen, service.WithSystemNodeToleration())
	switch {
	case err == nil:
		// OK.
	case common.ErrQuotaExceeded(err):
		s.recorder.Event(vi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
		return setQuotaExceededPhaseCondition(s.cb, &vi.Status.Phase, err, vi.CreationTimestamp), nil
	default:
		setPhaseConditionToFailed(s.cb, &vi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
		return nil, err
	}

	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vicondition.Provisioning).
		Message("Bounder pod has created: waiting to be Bound.")

	return nil, nil
}

func (s CreateBounderPodStep) isWFFC(ctx context.Context) (bool, error) {
	if s.pvc.Spec.StorageClassName == nil || *s.pvc.Spec.StorageClassName == "" {
		return false, nil
	}

	scKey := types.NamespacedName{Name: *s.pvc.Spec.StorageClassName}
	sc, err := object.FetchObject(ctx, scKey, s.client, &storagev1.StorageClass{})
	if err != nil {
		return false, fmt.Errorf("fetch storage class: %w", err)
	}

	if sc == nil || sc.VolumeBindingMode == nil || *sc.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		return false, nil
	}

	return true, nil
}
