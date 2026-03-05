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

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type CreateDataVolumeStepDiskService interface {
	Start(ctx context.Context, pvcSize resource.Quantity, sc *storagev1.StorageClass, source *cdiv1.DataVolumeSource, obj client.Object, sup supplements.DataVolumeSupplement, opts ...service.Option) error
}

type CreateDataVolumeStep struct {
	dv     *cdiv1.DataVolume
	disk   CreateDataVolumeStepDiskService
	client client.Client
	source *cdiv1.DataVolumeSource
	size   resource.Quantity
	cb     *conditions.ConditionBuilder
}

func NewCreateDataVolumeStep(
	dv *cdiv1.DataVolume,
	disk CreateDataVolumeStepDiskService,
	client client.Client,
	source *cdiv1.DataVolumeSource,
	size resource.Quantity,
	cb *conditions.ConditionBuilder,
) *CreateDataVolumeStep {
	return &CreateDataVolumeStep{
		dv:     dv,
		disk:   disk,
		client: client,
		source: source,
		size:   size,
		cb:     cb,
	}
}

func (s CreateDataVolumeStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if s.dv != nil {
		return nil, nil
	}

	supgen := vdsupplements.NewGenerator(vd)

	vd.Status.Progress = "0%"

	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.StorageClassName}, s.client, &storagev1.StorageClass{})
	if err != nil {
		return nil, fmt.Errorf("get sc: %w", err)
	}

	isWFFC := sc != nil && sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
	if isWFFC && len(vd.Status.AttachedToVirtualMachines) != 1 {
		vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
		return &reconcile.Result{}, nil
	}

	var nodePlacement *provisioner.NodePlacement
	nodePlacement, err = GetNodePlacement(ctx, s.client, vd)
	if err != nil {
		return nil, fmt.Errorf("failed to get importer tolerations: %w", err)
	}

	err = s.disk.Start(ctx, s.size, sc, s.source, vd, supgen, service.WithNodePlacement(nodePlacement))
	switch {
	case err == nil:
		// OK.
	case errors.Is(err, volumemode.ErrStorageProfileNotFound):
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("StorageProfile not found in the cluster: Please check a StorageProfile name in the cluster.")
		return &reconcile.Result{}, nil
	default:
		return nil, fmt.Errorf("start immediate: %w", err)
	}

	return nil, nil
}

func GetNodePlacement(ctx context.Context, c client.Client, vd *v1alpha2.VirtualDisk) (*provisioner.NodePlacement, error) {
	if len(vd.Status.AttachedToVirtualMachines) != 1 {
		return nil, nil
	}

	vmKey := types.NamespacedName{Name: vd.Status.AttachedToVirtualMachines[0].Name, Namespace: vd.Namespace}
	vm, err := object.FetchObject(ctx, vmKey, c, &v1alpha2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("unable to get the virtual machine %s: %w", vmKey, err)
	}

	if vm == nil {
		return nil, nil
	}

	var nodePlacement provisioner.NodePlacement
	nodePlacement.Tolerations = append(nodePlacement.Tolerations, vm.Spec.Tolerations...)

	vmClassKey := types.NamespacedName{Name: vm.Spec.VirtualMachineClassName}
	vmClass, err := object.FetchObject(ctx, vmClassKey, c, &v1alpha2.VirtualMachineClass{})
	if err != nil {
		return nil, fmt.Errorf("unable to get the virtual machine class %s: %w", vmClassKey, err)
	}

	if vmClass == nil {
		return &nodePlacement, nil
	}

	nodePlacement.Tolerations = append(nodePlacement.Tolerations, vmClass.Spec.Tolerations...)

	return &nodePlacement, nil
}
