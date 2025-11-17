/*
Copyright 2024 Flant JSC

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

package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type LifeCycleHandler struct {
	snapshotter LifeCycleSnapshotter
}

func NewLifeCycleHandler(snapshotter LifeCycleSnapshotter) *LifeCycleHandler {
	return &LifeCycleHandler{
		snapshotter: snapshotter,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("lifecycle"))

	cb := conditions.NewConditionBuilder(vdscondition.VirtualDiskSnapshotReadyType).Generation(vdSnapshot.Generation)

	defer func() { conditions.SetCondition(cb, &vdSnapshot.Status.Conditions) }()

	vs, err := h.snapshotter.GetVolumeSnapshot(ctx, vdSnapshot.Name, vdSnapshot.Namespace)
	if err != nil {
		setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if vdSnapshot.DeletionTimestamp != nil {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseTerminating
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")

		return reconcile.Result{}, nil
	}

	switch vdSnapshot.Status.Phase {
	case "":
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhasePending
	case v1alpha2.VirtualDiskSnapshotPhaseFailed:
		err := h.unfreezeFilesystemIfFailed(ctx, vdSnapshot)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return reconcile.Result{RequeueAfter: 5 * time.Second}, err
			}
			// Who changes the state? Sync?
			if errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition) {
				return reconcile.Result{}, nil
			}
			if cb.Condition().Message != "" {
				cb.Message(fmt.Sprintf("%s, %s", err.Error(), cb.Condition().Message))
			} else {
				cb.Message(err.Error())
			}
		}

		readyCondition, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vdSnapshot.Status.Conditions)
		cb.
			Status(metav1.ConditionFalse).
			Reason(conditions.CommonReason(readyCondition.Reason)).
			Message(readyCondition.Message)
		return reconcile.Result{}, nil
	case v1alpha2.VirtualDiskSnapshotPhaseReady:
		if vs == nil || vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.VolumeSnapshotLost).
				Message(fmt.Sprintf("The underlying volume snapshot %q is not ready to use.", vdSnapshot.Status.VolumeSnapshotName))
			return reconcile.Result{Requeue: true}, nil
		}

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
		vdSnapshot.Status.VolumeSnapshotName = vs.Name
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdscondition.VirtualDiskSnapshotReady).
			Message("")

		return reconcile.Result{}, nil
	}

	vd, err := h.snapshotter.GetVirtualDisk(ctx, vdSnapshot.Spec.VirtualDiskName, vdSnapshot.Namespace)
	if err != nil {
		setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	virtualDiskReadyCondition, _ := conditions.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
	if vd == nil || virtualDiskReadyCondition.Status != metav1.ConditionTrue {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.WaitingForTheVirtualDisk).
			Message(fmt.Sprintf("Waiting for the virtual disk %q to be ready for snapshotting.", vdSnapshot.Spec.VirtualDiskName))
		return reconcile.Result{}, nil
	}

	var pvc *corev1.PersistentVolumeClaim
	if vd.Status.Target.PersistentVolumeClaim != "" {
		pvc, err = h.snapshotter.GetPersistentVolumeClaim(ctx, vd.Status.Target.PersistentVolumeClaim, vd.Namespace)
		if err != nil {
			setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
			return reconcile.Result{}, err
		}
	}

	if pvc == nil || pvc.Status.Phase != corev1.ClaimBound {
		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.WaitingForTheVirtualDisk).
			Message("Waiting for the virtual disk's pvc to be in phase Bound.")
		return reconcile.Result{}, nil
	}

	vm, err := getVirtualMachine(ctx, vd, h.snapshotter)
	if err != nil {
		setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	kvvmi, err := h.snapshotter.GetVirtualMachineInstance(ctx, vm)
	if err != nil {
		setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	err = h.snapshotter.SyncFSFreezeRequest(ctx, kvvmi)
	switch {
	case err == nil:
		// OK.
	case errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition):
		return reconcile.Result{}, nil
	case k8serrors.IsConflict(err):
		return reconcile.Result{RequeueAfter: 5 * time.Second}, err
	default:
		return reconcile.Result{}, err
	}

	isFSFrozen, err := h.snapshotter.IsFrozen(kvvmi)
	if err != nil {
		setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
		return reconcile.Result{}, err
	}

	switch {
	case vs == nil:
		if vm != nil && vm.Status.Phase != v1alpha2.MachineStopped && !isFSFrozen {
			canFreeze, err := h.snapshotter.CanFreeze(ctx, kvvmi)
			if err != nil {
				return reconcile.Result{}, err
			}
			if canFreeze {
				log.Debug("Freeze the virtual machine to take a snapshot")

				if vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhasePending {
					vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
					cb.
						Status(metav1.ConditionFalse).
						Reason(vdscondition.Snapshotting).
						Message("The snapshotting process has started.")
					return reconcile.Result{Requeue: true}, nil
				}

				err = h.snapshotter.Freeze(ctx, kvvmi)
				if err != nil {
					if k8serrors.IsConflict(err) {
						return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
					}
					cb.
						Status(metav1.ConditionFalse).
						Reason(vdscondition.FileSystemFreezing).
						Message(service.CapitalizeFirstLetter(err.Error() + "."))
					return reconcile.Result{}, err
				}

				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress // TODO: add it to freeze/unfreeze block if required
				cb.
					Status(metav1.ConditionFalse).
					Reason(vdscondition.FileSystemFreezing).
					Message(fmt.Sprintf(
						"The virtual machine %q with an attached virtual disk %q is in the process of being frozen for taking a snapshot.",
						vm.Name, vdSnapshot.Spec.VirtualDiskName,
					))
				return reconcile.Result{}, nil
			}

			if vdSnapshot.Spec.RequiredConsistency {
				vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhasePending
				cb.
					Status(metav1.ConditionFalse).
					Reason(vdscondition.PotentiallyInconsistent)

				agentReadyCondition, _ := conditions.GetCondition(vmcondition.TypeAgentReady, vm.Status.Conditions)
				switch {
				case agentReadyCondition.Status != metav1.ConditionTrue:
					cb.Message(fmt.Sprintf(
						"The virtual machine %q with an attached virtual disk %q is %s: "+
							"the snapshotting of virtual disk might result in an inconsistent snapshot: "+
							"virtual machine agent is not ready and virtual machine cannot be frozen: "+
							"waiting for virtual machine agent to be ready, or virtual machine will stop",
						vm.Name, vd.Name, vm.Status.Phase,
					))
				default:
					cb.Message(fmt.Sprintf(
						"The virtual machine %q with an attached virtual disk %q is %s: "+
							"the snapshotting of virtual disk might result in an inconsistent snapshot: "+
							"waiting for the virtual machine to be %s or the disk to be detached",
						vm.Name, vd.Name, vm.Status.Phase, v1alpha2.MachineStopped,
					))
				}

				return reconcile.Result{}, nil
			}
		}

		if vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhasePending {
			vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.Snapshotting).
				Message("The snapshotting process has started.")
			return reconcile.Result{Requeue: true}, nil
		}

		log.Debug("The corresponding volume snapshot not found: create the new one")

		anno := make(map[string]string)
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
			anno[annotations.AnnStorageClassName] = *pvc.Spec.StorageClassName
		}

		if pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode != "" {
			anno[annotations.AnnVolumeMode] = string(*pvc.Spec.VolumeMode)
		}

		accessModes := make([]string, 0, len(pvc.Status.AccessModes))
		for _, accessMode := range pvc.Status.AccessModes {
			accessModes = append(accessModes, string(accessMode))
		}

		anno[annotations.AnnAccessModes] = strings.Join(accessModes, ",")

		if len(vd.Annotations) > 0 {
			vdAnnotationsJSON, err := json.Marshal(vd.Annotations)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to marshal VirtualDisk annotations: %w", err)
			} else {
				anno[annotations.AnnVirtualDiskOriginalAnnotations] = string(vdAnnotationsJSON)
			}
		}

		if len(vd.Labels) > 0 {
			vdLabelsJSON, err := json.Marshal(vd.Labels)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to marshal VirtualDisk labels: %w", err)
			} else {
				anno[annotations.AnnVirtualDiskOriginalLabels] = string(vdLabelsJSON)
			}
		}

		vs = &vsv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: anno,
				Name:        vdSnapshot.Name,
				Namespace:   vdSnapshot.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					service.MakeOwnerReference(vdSnapshot),
				},
			},
			Spec: vsv1.VolumeSnapshotSpec{
				Source: vsv1.VolumeSnapshotSource{
					PersistentVolumeClaimName: &pvc.Name,
				},
			},
		}

		vs, err = h.snapshotter.CreateVolumeSnapshot(ctx, vs)
		if err != nil {
			setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
			return reconcile.Result{}, err
		}

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
		vdSnapshot.Status.VolumeSnapshotName = vs.Name
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.Snapshotting).
			Message(fmt.Sprintf("The snapshotting process for virtual disk %q has started.", vdSnapshot.Spec.VirtualDiskName))
		return reconcile.Result{}, nil
	case vs.Status != nil && vs.Status.Error != nil && vs.Status.Error.Message != nil:
		log.Debug("The volume snapshot has an error")

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.VirtualDiskSnapshotFailed).
			Message(fmt.Sprintf("VolumeSnapshot %q has an error: %s.", vs.Name, *vs.Status.Error.Message))
		return reconcile.Result{}, nil
	case vs.Status == nil || vs.Status.ReadyToUse == nil || !*vs.Status.ReadyToUse:
		log.Debug("Waiting for the volume snapshot to be ready to use")

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseInProgress
		vdSnapshot.Status.VolumeSnapshotName = vs.Name
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.Snapshotting).
			Message(fmt.Sprintf("Waiting fot the volume snapshot %q to be ready to use.", vdSnapshot.Name))
		return reconcile.Result{}, nil
	default:
		log.Debug("The volume snapshot is ready to use")

		if h.isConsistent(vdSnapshot) {
			err := h.unfreezeFilesystem(ctx, vdSnapshot.Name, vm, kvvmi)
			if err != nil {
				if k8serrors.IsConflict(err) {
					return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
				}
				return reconcile.Result{}, err
			}
		}

		if vdSnapshot.Status.Consistent == nil {
			if vm == nil || vm.Status.Phase == v1alpha2.MachineStopped || isFSFrozen {
				vdSnapshot.Status.Consistent = ptr.To(true)
				return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
			}

			if vdSnapshot.Spec.RequiredConsistency {
				err = fmt.Errorf("virtual disk snapshot is not consistent because the virtual machine %s has not been stopped or its filesystem has not been frozen", vm.Name)
				setPhaseConditionToFailed(cb, &vdSnapshot.Status.Phase, err)
				return reconcile.Result{}, err
			}
		}

		err = h.unfreezeFilesystem(ctx, vdSnapshot.Name, vm, kvvmi)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
			}
			return reconcile.Result{}, err
		}

		err = h.snapshotter.SyncFSFreezeRequest(ctx, kvvmi)
		switch {
		case err == nil:
			// OK.
		case errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition):
			return reconcile.Result{}, nil
		case k8serrors.IsConflict(err):
			return reconcile.Result{RequeueAfter: 5 * time.Second}, err
		default:
			return reconcile.Result{}, err
		}

		vdSnapshot.Status.Phase = v1alpha2.VirtualDiskSnapshotPhaseReady
		vdSnapshot.Status.VolumeSnapshotName = vs.Name
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdscondition.VirtualDiskSnapshotReady).
			Message("")

		return reconcile.Result{}, nil
	}
}

func getVirtualMachine(ctx context.Context, vd *v1alpha2.VirtualDisk, snapshotter LifeCycleSnapshotter) (*v1alpha2.VirtualMachine, error) {
	if vd == nil {
		return nil, nil
	}

	// TODO: ensure vd.Status.AttachedToVirtualMachines is in the actual state.
	switch len(vd.Status.AttachedToVirtualMachines) {
	case 0:
		return nil, nil
	case 1:
		vm, err := snapshotter.GetVirtualMachine(ctx, vd.Status.AttachedToVirtualMachines[0].Name, vd.Namespace)
		if err != nil {
			return nil, err
		}

		return vm, nil
	default:
		return nil, fmt.Errorf("the virtual disk %q is attached to multiple virtual machines", vd.Name)
	}
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *v1alpha2.VirtualDiskSnapshotPhase, err error) {
	*phase = v1alpha2.VirtualDiskSnapshotPhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vdscondition.VirtualDiskSnapshotFailed).
		Message(service.CapitalizeFirstLetter(err.Error() + "."))
}

func (h LifeCycleHandler) unfreezeFilesystemIfFailed(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot) error {
	vd, err := h.snapshotter.GetVirtualDisk(ctx, vdSnapshot.Spec.VirtualDiskName, vdSnapshot.Namespace)
	if err != nil {
		return err
	}

	if vd == nil {
		return nil
	}

	vm, err := getVirtualMachine(ctx, vd, h.snapshotter)
	if err != nil {
		return err
	}

	if vm == nil {
		return nil
	}

	kvvmi, err := h.snapshotter.GetVirtualMachineInstance(ctx, vm)
	if err != nil {
		return err
	}

	err = h.unfreezeFilesystem(ctx, vdSnapshot.Name, vm, kvvmi)
	if err != nil {
		return err
	}

	return nil
}

func (h LifeCycleHandler) unfreezeFilesystem(ctx context.Context, vdSnapshotName string, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) error {
	canUnfreeze, err := h.snapshotter.CanUnfreezeWithVirtualDiskSnapshot(ctx, vdSnapshotName, vm, kvvmi)
	if err != nil {
		return err
	}

	if canUnfreeze {
		err = h.snapshotter.Unfreeze(ctx, kvvmi)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h LifeCycleHandler) isConsistent(vdSnapshot *v1alpha2.VirtualDiskSnapshot) bool {
	if vdSnapshot.Status.Consistent == nil {
		return false
	}

	if !*vdSnapshot.Status.Consistent {
		return false
	}

	return true
}
