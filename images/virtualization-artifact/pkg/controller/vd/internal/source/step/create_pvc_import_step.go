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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

// PVCImportStepDiskService is the slice of DiskService that the step needs to
// resolve volume/access modes for the target PVC.
type PVCImportStepDiskService interface {
	VolumeAndAccessModesGetter
}

// PVCService is the contract every PVC-import step relies on to create and
// populate a target PersistentVolumeClaim.
type PVCService interface {
	CreateTargetFromDVCR(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, source *service.PVCImportSourceRegistry, modeGetter service.VolumeAndAccessModesGetter, nodePlacement *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error)
	CreateTargetFromPVC(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, source *corev1.PersistentVolumeClaim, modeGetter service.VolumeAndAccessModesGetter, nodePlacement *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error)
	CreateTargetFromVS(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, source *vsv1.VolumeSnapshot, modeGetter service.VolumeAndAccessModesGetter, nodePlacement *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error)
	Import(ctx context.Context, target *corev1.PersistentVolumeClaim, source *service.PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) error
	WaitForImport(ctx context.Context, target *corev1.PersistentVolumeClaim, source *service.PVCImportSource, owner client.Object, sup supplements.Generator, nodePlacement *provisioner.NodePlacement) (corev1.PodPhase, error)
}

// PVCImportStep is the step that initiates target PVC creation for a
// VirtualDisk import. It explicitly chooses the target creation method from the
// import source and creates the target PVC with the annotations/dataSource
// needed by the wait step.
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

func NewCreatePVCStep(
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

	nodePlacement, err := commonvd.GetNodePlacement(ctx, s.client, vd)
	if err != nil {
		return nil, fmt.Errorf("failed to get importer tolerations: %w", err)
	}

	// On a WaitForFirstConsumer storage class the target PVC is created as soon
	// as a VirtualMachine consumes the disk, before the VM is scheduled. This is
	// safe: the foreign dataSourceRef defers CSI provisioning, and the populator
	// starts the import only once the scheduler has stamped the selected-node
	// annotation — set for whatever pod consumes the PVC first (the VM's
	// launcher, KubeVirt's temporary first-consumer pod, or a hotplug attachment
	// pod) — pinning the prime PVC to that node. Waiting for the VM's node here
	// instead would deadlock paravirtualization-disabled VMs: their disks are
	// static volumes, and KubeVirt refuses to create the VM pod until the PVC
	// exists.

	key := types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}
	if err := s.createTarget(ctx, key, sc.Name, vd, nodePlacement); err != nil {
		if errors.Is(err, volumemode.ErrStorageProfileNotFound) {
			vd.Status.Phase = v1alpha2.DiskFailed
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ProvisioningFailed).
				Message("The StorageClass is not fully configured in the cluster. Check the StorageClass name or set a default StorageClass.")
			return &reconcile.Result{}, nil
		}
		// A project quota that rejects the target PVC is a recoverable, user-facing
		// condition rather than a controller error: surface it on the Ready condition
		// (and requeue via the resource-quota watcher) instead of returning an error
		// that would be logged as a "Reconciler error".
		if common.ErrQuotaExceeded(err) {
			vd.Status.Phase = v1alpha2.DiskPending
			vd.Status.Progress = ""
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.QuotaExceeded).
				Message(fmt.Sprintf("Quota exceeded during the creation of the target PersistentVolumeClaim: %s", err))
			return &reconcile.Result{}, nil
		}
		return nil, fmt.Errorf("create target pvc: %w", err)
	}

	return nil, nil
}

func (s PVCImportStep) createTarget(ctx context.Context, key types.NamespacedName, storageClassName string, vd *v1alpha2.VirtualDisk, nodePlacement *provisioner.NodePlacement) error {
	switch {
	case s.source == nil:
		return nil
	case s.source.Registry != nil:
		_, err := s.pvc.CreateTargetFromDVCR(ctx, key, storageClassName, &s.size, vd, s.source.Registry, s.disk, nodePlacement)
		return err
	case s.source.PVC != nil:
		sourceClaim, err := object.FetchObject(ctx, types.NamespacedName{Name: s.source.PVC.Name, Namespace: s.source.PVC.Namespace}, s.client, &corev1.PersistentVolumeClaim{})
		if err != nil {
			return fmt.Errorf("fetch source pvc: %w", err)
		}
		if sourceClaim == nil {
			return fmt.Errorf("source pvc %s/%s not found", s.source.PVC.Namespace, s.source.PVC.Name)
		}
		_, err = s.pvc.CreateTargetFromPVC(ctx, key, storageClassName, &s.size, vd, sourceClaim, s.disk, nodePlacement)
		return err
	default:
		return nil
	}
}
