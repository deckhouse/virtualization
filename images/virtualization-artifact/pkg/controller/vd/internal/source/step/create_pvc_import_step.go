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
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

// PVCImportStepDiskService is the slice of DiskService that the step needs to
// resolve volume/access modes for the target PVC.
type PVCImportStepDiskService interface {
	VolumeAndAccessModesGetter
}

// PVCService is the contract every PVC-import step relies on to actually
// populate a target PersistentVolumeClaim. Steps build the target PVC
// descriptor (metadata + base spec) and hand the rest off to Import; the
// service decides whether to use a smart clone (Snapshot / CSI) or the
// cdi-importer pod, and creates every helper resource it needs.
type PVCService interface {
	Finalizers() []string
	Import(ctx context.Context, target *corev1.PersistentVolumeClaim, source *service.PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error)
}

// PVCImportStep is the step that initiates target PVC creation for a
// VirtualDisk import. It assembles the target PVC descriptor (Name,
// Namespace, OwnerReferences, Finalizers, base Spec) and calls
// PersistentVolumeClaimService.Import, which then determines the import
// strategy and provisions every underlying resource needed (scratch PVC,
// cdi-importer pod, DVCR auth/CA copies, VolumeSnapshot, etc.).
//
// The step is idempotent: subsequent invocations are no-ops once the target
// PVC already exists.
type PVCImportStep struct {
	disk   PVCImportStepDiskService
	pvc    PVCService
	client client.Client
	source *service.PVCImportSource
	size   resource.Quantity
	cb     *conditions.ConditionBuilder
}

func NewPVCImportStep(
	disk PVCImportStepDiskService,
	pvcSvc PVCService,
	c client.Client,
	source *service.PVCImportSource,
	size resource.Quantity,
	cb *conditions.ConditionBuilder,
) *PVCImportStep {
	return &PVCImportStep{
		disk:   disk,
		pvc:    pvcSvc,
		client: c,
		source: source,
		size:   size,
		cb:     cb,
	}
}

func (s PVCImportStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	if vd.Status.Progress == "" {
		vd.Status.Progress = "0%"
	}

	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: vd.Status.StorageClassName}, s.client, &storagev1.StorageClass{})
	if err != nil {
		return nil, fmt.Errorf("get sc: %w", err)
	}

	isWFFC := sc != nil && sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
	if isWFFC && len(vd.Status.AttachedToVirtualMachines) != 1 {
		vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.WaitingForFirstConsumer).
			Message("Awaiting the creation and scheduling of the VirtualMachine with the attached VirtualDisk.")
		return &reconcile.Result{}, nil
	}

	if sc == nil {
		return nil, fmt.Errorf("storage class %q not found", vd.Status.StorageClassName)
	}

	volumeMode, accessMode, err := s.disk.GetVolumeAndAccessModes(ctx, vd, sc)
	switch {
	case err == nil:
	case errors.Is(err, volumemode.ErrStorageProfileNotFound):
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("StorageProfile not found in the cluster: Please check a StorageProfile name in the cluster.")
		return &reconcile.Result{}, nil
	default:
		return nil, fmt.Errorf("get volume and access modes: %w", err)
	}

	nodePlacement, err := GetNodePlacement(ctx, s.client, vd)
	if err != nil {
		return nil, fmt.Errorf("failed to get importer tolerations: %w", err)
	}

	target := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:       vd.Status.Target.PersistentVolumeClaim,
			Namespace:  vd.Namespace,
			Finalizers: s.pvc.Finalizers(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         v1alpha2.SchemeGroupVersion.String(),
				Kind:               v1alpha2.VirtualDiskKind,
				Name:               vd.Name,
				UID:                vd.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: *pvc.CreateSpec(&sc.Name, s.size, accessMode, volumeMode),
	}

	sup := vdsupplements.NewGenerator(vd)
	if _, err := s.pvc.Import(ctx, target, s.source, vd, sup, nodePlacement); err != nil {
		return nil, fmt.Errorf("pvc import: %w", err)
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
