package controller

import (
	"context"
	"fmt"
	"strings"
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
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvapi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
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
	if state.isDeletion() {
		// VM may be deleted before deleting VMBDA or disk may not be attached.
		if state.VM != nil && isAttached(state) {
			opts.Log.Info("Start volume detaching", "vmbda.name", state.VMBDA.Current().Name)

			err := r.unhotplugVolume(ctx, state)
			if err != nil {
				return err
			}

			if r.removeVMHotpluggedLabel(state) {
				err := opts.Client.Update(ctx, state.VM)
				if err != nil {
					return fmt.Errorf("failed to remove VM labels with hotplugged block device %s: %w", state.VMD.Name, err)
				}
			}

			if state.RemoveVMStatusBDA() {
				if err := opts.Client.Status().Update(ctx, state.VM); err != nil {
					return fmt.Errorf("failed to remove attached disk %s: %w", state.VMD.Name, err)
				}
			}
			opts.Log.Info("Volume detached", "vmbda.name", state.VMBDA.Current().Name, "vm.name", state.VM.Name)
		}

		controllerutil.RemoveFinalizer(state.VMBDA.Changed(), virtv2.FinalizerVMBDACleanup)

		return nil
	}

	// Set finalizer atomically using requeue.
	if controllerutil.AddFinalizer(state.VMBDA.Changed(), virtv2.FinalizerVMBDACleanup) {
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	}

	// Do nothing if VM not found or not running.
	if state.VM == nil {
		opts.Log.V(1).Info(fmt.Sprintf("VM %s is not created, do nothing", state.VMBDA.Current().Spec.VMName))
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VM is missing")
		return nil
	}

	if state.VM.Status.Phase != virtv2.MachineRunning {
		opts.Log.V(1).Info(fmt.Sprintf("VM %s is not running yet, do nothing", state.VMBDA.Current().Spec.VMName))
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VM is not Running")
		return nil
	}

	// Do nothing if VM not found or not running.
	if state.KVVMI == nil {
		opts.Log.V(1).Info(fmt.Sprintf("KVVMI for VM %s is absent, do nothing", state.VMBDA.Current().Spec.VMName))
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VM is missing")
		return nil
	}

	blockDeviceIndex := state.IndexVMStatusBDA()

	// VM is running and disk is valid. Attach volume if not attached yet.
	if !isAttached(state) && blockDeviceIndex == -1 {
		opts.Log.Info("Start volume attaching")

		// Do nothing if KVVMI is not running.
		if state.KVVMI.Status.Phase != virtv1.Running {
			opts.Log.V(1).Info(fmt.Sprintf("KVVMI for VM %s is not running yet, do nothing", state.VMBDA.Current().Spec.VMName))
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VM is not Running")
			return nil
		}

		// Wait for hotplug possibility.
		hotplugMessage := r.checkHotplugSanity(state)
		if hotplugMessage != "" {
			opts.Log.Error(fmt.Errorf("hotplug not possible: %s", hotplugMessage), "")
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			state.SetStatusFailure(virtv2.ReasonHotplugPostponed, hotplugMessage)
			return nil
		}

		err := r.hotplugVolume(ctx, state)
		if err != nil {
			return err
		}

		opts.Log.Info("Volume attached")

		// Add attached device to the VM status.
		state.VM.Status.BlockDevicesAttached = append(state.VM.Status.BlockDevicesAttached, virtv2.BlockDeviceStatus{
			Type: virtv2.DiskDevice,
			VirtualMachineDisk: &virtv2.DiskDeviceSpec{
				Name: state.VMD.Name,
			},
			Target:       "",
			Size:         state.VMD.Status.Capacity,
			Hotpluggable: true,
		})

		if err := opts.Client.Status().Update(ctx, state.VM); err != nil {
			return fmt.Errorf("failed to add new attached block device %s: %w", state.VMD.Name, err)
		}
	}

	if !isAttached(state) {
		// Wait until attached to the KVVMI to update Status.Target.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	}

	if r.setVMHotpluggedLabel(state) {
		err := opts.Client.Update(ctx, state.VM)
		if err != nil {
			return fmt.Errorf("failed to set VM labels with hotplugged block device %s: %w", state.VMD.Name, err)
		}
	}

	if r.setVMHotpluggedFinalizer(state) {
		err := opts.Client.Update(ctx, state.VMD)
		if err != nil {
			return fmt.Errorf("failed to set VMD finalizer with hotplugged block device %s: %w", state.VMD.Name, err)
		}
	}

	if r.setVMStatusBlockDevicesAttached(blockDeviceIndex, state) {
		err := opts.Client.Status().Update(ctx, state.VM)
		if err != nil {
			return fmt.Errorf("failed to update VM status with hotplugged block device %s: %w", state.VMD.Name, err)
		}
	}

	return nil
}

func (r *VMBDAReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMBDAReconcilerState, _ two_phase_reconciler.ReconcilerOptions) error {
	// Do nothing if object is being deleted as any update will lead to en error.
	if state.isDeletion() {
		return nil
	}

	vmBdaStatus := state.VMBDA.Current().Status.DeepCopy()

	// TODO isFailed returns false-positive condition that is not related to VMBDA disk.
	// TODO Filter this condition and move 'case isAttached' before 'case reason'.
	reason, message := isFailed(state)
	vmBdaStatus.FailureReason = reason
	vmBdaStatus.FailureMessage = message
	vmBdaStatus.VMName = state.VMBDA.Current().Spec.VMName

	switch {
	case isAttached(state):
		vmBdaStatus.Phase = virtv2.BlockDeviceAttachmentPhaseAttached
	case reason != "":
		vmBdaStatus.Phase = virtv2.BlockDeviceAttachmentPhaseFailed
	default:
		vmBdaStatus.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
	}

	state.VMBDA.Changed().Status = *vmBdaStatus

	return nil
}

// isFailed return reason and message either from SetFailure or from
// DisksNotLiveMigratable condition in the underlying kubevirt VMI.
func isFailed(state *VMBDAReconcilerState) (string, string) {
	reason := state.FailureReason
	message := state.FailureMessage

	if reason != "" && message != "" {
		return reason, message
	}

	for _, condition := range state.KVVMI.Status.Conditions {
		if condition.Type == virtv1.VirtualMachineInstanceIsMigratable {
			if condition.Status == corev1.ConditionFalse && condition.Reason == virtv1.VirtualMachineInstanceReasonDisksNotMigratable {
				reason = condition.Reason
				message = condition.Message
			}
			break
		}
	}

	return reason, message
}

func isAttached(state *VMBDAReconcilerState) bool {
	for _, status := range state.KVVMI.Status.VolumeStatus {
		if status.Name == kvbuilder.GenerateVMDDiskName(state.VMD.Name) {
			return status.Phase == virtv1.VolumeReady
		}
	}

	return false
}

// hotplugVolume requests kubevirt subresources APIService to attach volume
// to KVVMI.
func (r *VMBDAReconciler) hotplugVolume(ctx context.Context, state *VMBDAReconcilerState) error {
	if state.VMBDA.Current().Spec.BlockDevice.Type != virtv2.BlockDeviceAttachmentTypeVirtualMachineDisk {
		return fmt.Errorf("unknown block device attachment type %s", state.VMBDA.Current().Spec.BlockDevice.Type)
	}
	name := kvbuilder.GenerateVMDDiskName(state.VMBDA.Current().Spec.BlockDevice.VirtualMachineDisk.Name)
	hotplugRequest := virtv1.AddVolumeOptions{
		Name: name,
		Disk: &virtv1.Disk{
			Name: name,
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
		Name: kvbuilder.GenerateVMDDiskName(state.VMBDA.Current().Spec.BlockDevice.VirtualMachineDisk.Name),
	}

	err := r.kubevirt.RemoveVolume(ctx, state.VMBDA.Current().Namespace, state.VMBDA.Current().Spec.VMName, &unhotplugRequest)
	if err != nil {
		return fmt.Errorf("error removing volume, %w", err)
	}

	return nil
}

func (r *VMBDAReconciler) setVMHotpluggedLabel(state *VMBDAReconcilerState) bool {
	labels := state.VM.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	label := vmattachee.MakeHotpluggedResourceLabelKeyFormat("vmd", state.VMD.Name)
	_, ok := labels[label]
	if ok {
		return false
	}

	labels[label] = vmattachee.HotpluggedLabelValue

	state.VM.SetLabels(labels)

	return true
}

func (r *VMBDAReconciler) removeVMHotpluggedLabel(state *VMBDAReconcilerState) bool {
	labels := state.VM.GetLabels()
	if labels == nil {
		return false
	}

	label := vmattachee.MakeHotpluggedResourceLabelKeyFormat("vmd", state.VMD.Name)
	_, ok := labels[label]
	if !ok {
		return false
	}

	delete(labels, label)

	state.VM.SetLabels(labels)

	return true
}

func (r *VMBDAReconciler) setVMHotpluggedFinalizer(state *VMBDAReconcilerState) bool {
	return controllerutil.AddFinalizer(state.VMD, virtv2.FinalizerVMDProtection)
}

// setVMStatusBlockDevicesAttached copy volume status from KVVMI for attached disk to the d8 VM block devices status.
func (r *VMBDAReconciler) setVMStatusBlockDevicesAttached(blockDeviceIndex int, state *VMBDAReconcilerState) bool {
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

			return true
		}

		return false
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

	return true
}

// checkHotplugSanity detects if it is possible to hotplug disk to the VM.
// 1. It searches for disk in VM spec and returns false if disk is already attached to VM.
// 2. It returns false if VM is in the "Manual approve" mode.
func (r *VMBDAReconciler) checkHotplugSanity(state *VMBDAReconcilerState) string {
	if state.VM == nil {
		return ""
	}

	messages := make([]string, 0)

	// Check if disk is already in the VM.
	diskName := state.VMBDA.Current().Spec.BlockDevice.VirtualMachineDisk.Name

	for _, bd := range state.VM.Spec.BlockDevices {
		disk := bd.VirtualMachineDisk
		if disk != nil && disk.Name == diskName {
			messages = append(messages, fmt.Sprintf("disk %s is already attached to VM", diskName))
			break
		}
	}

	if state.VM.Annotations[cc.AnnVMChangeID] != "" {
		messages = append(messages, "vm waits for changes approval")
	}

	if len(messages) == 0 {
		return ""
	}

	return strings.Join(messages, ", ")
}
