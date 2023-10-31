package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvapi"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type VMBDAReconciler struct {
	kubevirt *kvapi.Client
}

func NewVMBDAReconciler(kv *kvapi.Client) *VMBDAReconciler {
	return &VMBDAReconciler{
		kubevirt: kv,
	}
}

func (r *VMBDAReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineBlockDeviceAttachment{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	)
}

func (r *VMBDAReconciler) Sync(ctx context.Context, _ reconcile.Request, state *VMBDAReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	blockDeviceIndex := -1
	for i, blockDevice := range state.VM.Status.BlockDevicesAttached {
		if blockDevice.VirtualMachineDisk != nil && blockDevice.VirtualMachineDisk.Name == state.VMD.Name {
			blockDeviceIndex = i
			break
		}
	}

	switch {
	case state.isDeletion():
		opts.Log.Info("Start volume detaching")

		err := r.unhotplugVolume(ctx, state)
		if err != nil {
			return err
		}

		if blockDeviceIndex > -1 {
			state.VM.Status.BlockDevicesAttached = append(
				state.VM.Status.BlockDevicesAttached[:blockDeviceIndex],
				state.VM.Status.BlockDevicesAttached[blockDeviceIndex+1:]...,
			)

			if err = opts.Client.Status().Update(ctx, state.VM); err != nil {
				return fmt.Errorf("failed to remove attached disk %s: %w", state.VMD.Name, err)
			}
		}

		if state.isProtected() {
			controllerutil.RemoveFinalizer(state.VMBDA.Changed(), virtv2.FinalizerVMBDACleanup)
		}

		opts.Log.Info("Volume detached")

		return nil
	case state.VMBDA.Current().Status.Phase == "":
		if !state.isProtected() {
			controllerutil.AddFinalizer(state.VMBDA.Changed(), virtv2.FinalizerVMBDACleanup)
		}

		opts.Log.Info("Start volume attaching")

		if blockDeviceIndex > -1 {
			return errors.New("VirtualMachineDisk already hotplugged to VirtualMachine")
		}

		err := r.hotplugVolume(ctx, state)
		if err != nil {
			return err
		}

		opts.Log.Info("Volume attached")
	}

	var vs virtv1.VolumeStatus

	for i := range state.KVVMI.Status.VolumeStatus {
		if state.KVVMI.Status.VolumeStatus[i].Name == state.VMD.Name {
			vs = state.KVVMI.Status.VolumeStatus[i]
		}
	}

	if blockDeviceIndex > -1 {
		blockDevice := state.VM.Status.BlockDevicesAttached[blockDeviceIndex]
		if blockDevice.Target != vs.Target || blockDevice.Size != state.VMD.Status.Capacity {
			blockDevice.Target = vs.Target
			blockDevice.Size = state.VMD.Status.Capacity

			state.VM.Status.BlockDevicesAttached[blockDeviceIndex] = blockDevice

			if err := opts.Client.Status().Update(ctx, state.VM); err != nil {
				return fmt.Errorf("failed to update attached block device %s: %w", state.VM.Status.BlockDevicesAttached[blockDeviceIndex].VirtualMachineDisk.Name, err)
			}
		}

		return nil
	}

	state.VM.Status.BlockDevicesAttached = append(state.VM.Status.BlockDevicesAttached, virtv2.BlockDeviceStatus{
		Type: virtv2.DiskDevice,
		VirtualMachineDisk: &virtv2.DiskDeviceSpec{
			Name: state.VMD.Name,
		},
		Target:       vs.Target,
		Size:         state.VMD.Status.Capacity,
		Hotpluggable: true,
	})

	if err := opts.Client.Status().Update(ctx, state.VM); err != nil {
		return fmt.Errorf("failed to add new attached block device %s: %w", state.VMD.Name, err)
	}

	return nil
}

func (r *VMBDAReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMBDAReconcilerState, _ two_phase_reconciler.ReconcilerOptions) error {
	// Do nothing if object is being deleted as any update will lead to en error.
	if state.isDeletion() {
		return nil
	}

	vmBdaStatus := state.VMBDA.Current().Status.DeepCopy()

	reason, message := isFailed(state)
	vmBdaStatus.FailureReason = reason
	vmBdaStatus.FailureMessage = message

	switch {
	case reason != "":
		vmBdaStatus.Phase = virtv2.BlockDeviceAttachmentPhaseFailed
	case isAttached(state):
		vmBdaStatus.Phase = virtv2.BlockDeviceAttachmentPhaseAttached
	default:
		vmBdaStatus.VMName = state.VMBDA.Current().Spec.VMName
		vmBdaStatus.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress

		// Requeue to wait until Pod become Running.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
	}

	state.VMBDA.Changed().Status = *vmBdaStatus

	return nil
}

func isFailed(state *VMBDAReconcilerState) (string, string) {
	for _, condition := range state.KVVMI.Status.Conditions {
		if condition.Type == virtv1.VirtualMachineInstanceIsMigratable {
			if condition.Status == corev1.ConditionFalse && condition.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable {
				return condition.Reason, condition.Message
			}

			return "", ""
		}
	}

	return "", ""
}

func isAttached(state *VMBDAReconcilerState) bool {
	for _, status := range state.KVVMI.Status.VolumeStatus {
		if status.Name == state.VMD.Name {
			return status.Phase == virtv1.VolumeReady
		}
	}

	return false
}

func (r *VMBDAReconciler) hotplugVolume(ctx context.Context, state *VMBDAReconcilerState) error {
	if state.VMBDA.Current().Spec.BlockDevice.Type != virtv2.BlockDeviceAttachmentTypeVirtualMachineDisk {
		return fmt.Errorf("unknown block device attachment type %s", state.VMBDA.Current().Spec.BlockDevice.Type)
	}

	hotplugRequest := virtv1.AddVolumeOptions{
		Name: state.VMBDA.Current().Spec.BlockDevice.VirtualMachineDisk.Name,
		Disk: &virtv1.Disk{
			Name: state.VMBDA.Current().Spec.BlockDevice.VirtualMachineDisk.Name,
			DiskDevice: virtv1.DiskDevice{
				Disk: &virtv1.DiskTarget{
					Bus: "scsi",
				},
			},
			Serial: state.VMBDA.Current().Spec.BlockDevice.VirtualMachineDisk.Name,
		},
		VolumeSource: &virtv1.HotplugVolumeSource{
			PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: state.PVC.Name,
				},
				Hotpluggable: true,
			},
		},
	}

	err := r.kubevirt.AddVolume(ctx, state.VMBDA.Current().Namespace, state.VMBDA.Current().Spec.VMName, hotplugRequest)
	if err != nil {
		return fmt.Errorf("error adding volume, %w", err)
	}

	return nil
}

func (r *VMBDAReconciler) unhotplugVolume(ctx context.Context, state *VMBDAReconcilerState) error {
	if state.VMBDA.Current().Spec.BlockDevice.Type != virtv2.BlockDeviceAttachmentTypeVirtualMachineDisk {
		return fmt.Errorf("unknown block device attachment type %s", state.VMBDA.Current().Spec.BlockDevice.Type)
	}

	unhotplugRequest := virtv1.RemoveVolumeOptions{
		Name: state.VMBDA.Current().Spec.BlockDevice.VirtualMachineDisk.Name,
	}

	err := r.kubevirt.RemoveVolume(ctx, state.VMBDA.Current().Namespace, state.VMBDA.Current().Spec.VMName, &unhotplugRequest)
	if err != nil {
		return fmt.Errorf("error removing volume, %w", err)
	}

	return nil
}
