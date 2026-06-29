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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type EnsureNodePlacementStepDiskService interface {
	CheckProvisioning(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error
	CleanUp(ctx context.Context, sup supplements.Generator) (bool, error)
}

// EnsureNodePlacementStep supports changing the node placement only if the PVC is created using a DataVolume.
type EnsureNodePlacementStep struct {
	pvc    *corev1.PersistentVolumeClaim
	dv     *cdiv1.DataVolume
	disk   EnsureNodePlacementStepDiskService
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewEnsureNodePlacementStep(
	pvc *corev1.PersistentVolumeClaim,
	dv *cdiv1.DataVolume,
	disk EnsureNodePlacementStepDiskService,
	client client.Client,
	cb *conditions.ConditionBuilder,
) *EnsureNodePlacementStep {
	return &EnsureNodePlacementStep{
		pvc:    pvc,
		dv:     dv,
		disk:   disk,
		client: client,
		cb:     cb,
	}
}

func (s EnsureNodePlacementStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.pvc == nil {
		return nil, nil
	}

	_, exists := vd.Annotations[annotations.AnnUseVolumeSnapshot]
	if exists {
		return nil, nil
	}

	err := s.disk.CheckProvisioning(ctx, s.pvc)
	switch {
	case err == nil:
		// OK.
		return nil, nil
	case errors.Is(err, service.ErrDataVolumeProvisionerUnschedulable):
		// Will be processed below.
	default:
		return nil, fmt.Errorf("check provisioning: %w", err)
	}

	nodePlacement, err := GetNodePlacement(ctx, s.client, vd)
	if err != nil {
		return nil, fmt.Errorf("get node placement: %w", err)
	}

	isChanged, err := provisioner.IsNodePlacementChanged(nodePlacement, s.dv)
	if err != nil {
		return nil, fmt.Errorf("is node placement changed: %w", err)
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning

	if !isChanged {
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("Trying to schedule the PersistentVolumeClaim provisioner.")
		return &reconcile.Result{}, nil
	}

	supgen := vdsupplements.NewGenerator(vd)

	_, err = s.disk.CleanUp(ctx, supgen)
	if err != nil {
		return nil, fmt.Errorf("clean up due to changes in the virtual machine tolerations: %w", err)
	}

	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("The PersistentVolumeClaim provisioner will be recreated due to changes in the virtual machine tolerations.")
	return &reconcile.Result{}, nil
}
