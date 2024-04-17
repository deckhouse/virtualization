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

	"github.com/deckhouse/virtualization-controller/pkg/controller/kubevirt"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvapi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMBDAReconciler struct {
	controllerNamespace string
}

func NewVMBDAReconciler(controllerNamespace string) *VMBDAReconciler {
	return &VMBDAReconciler{
		controllerNamespace: controllerNamespace,
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

			err := r.unplugVolume(ctx, state)
			if err != nil {
				return err
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
		opts.Log.V(1).Info(fmt.Sprintf("VM %s is not created, do nothing", state.VMBDA.Current().Spec.VirtualMachine))
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VM is missing")
		return nil
	}

	if state.VM.Status.Phase != virtv2.MachineRunning {
		opts.Log.V(1).Info(fmt.Sprintf("VM %s is not running yet, do nothing", state.VMBDA.Current().Spec.VirtualMachine))
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VM is not Running")
		return nil
	}

	// Do nothing if VM not found or not running.
	if state.KVVMI == nil {
		opts.Log.V(1).Info(fmt.Sprintf("KVVMI for VM %s is absent, do nothing", state.VMBDA.Current().Spec.VirtualMachine))
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VM is missing")
		return nil
	}

	// Do nothing if KVVMI is not running.
	if state.KVVMI.Status.Phase != virtv1.Running {
		opts.Log.V(1).Info(fmt.Sprintf("KVVMI for VM %s is not running yet, do nothing", state.VMBDA.Current().Spec.VirtualMachine))
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VM is not Running")
		return nil
	}

	// Do nothing if VMD not found or not running.
	if state.VMD == nil || state.VMD.Status.Phase != virtv2.DiskReady {
		opts.Log.V(1).Info("VMD is not ready yet, do nothing")
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "VMD is not ready")
		return nil
	}

	// Do nothing if PVC not found or not running.
	if state.PVC == nil || state.PVC.Status.Phase != corev1.ClaimBound {
		opts.Log.V(1).Info("PVC is not bound yet, do nothing")
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		state.SetStatusFailure(virtv2.ReasonHotplugPostponed, "PVC is not bound")
		return nil
	}

	blockDeviceIndex := state.IndexVMStatusBDA()

	// VM is running and disk is valid. Attach volume if not attached yet.
	if !isAttached(state) && blockDeviceIndex == -1 {
		opts.Log.Info("Start volume attaching")

		// Wait for hotplug possibility.
		hotplugMessage, ok := r.checkHotplugSanity(state)
		if !ok {
			opts.Log.Error(fmt.Errorf("hotplug not possible: %s", hotplugMessage), "")
			state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
			state.SetStatusFailure(virtv2.ReasonHotplugPostponed, hotplugMessage)
			return nil
		}

		err := r.hotplugVolume(ctx, state)
		if err != nil {
			return err
		}

		// Add attached device to the VM status.
		if r.setVMStatusBlockDeviceRefs(blockDeviceIndex, state) {
			err = opts.Client.Status().Update(ctx, state.VM)
			if err != nil {
				return fmt.Errorf("failed to update VM status with hotplugged block device %s: %w", state.VMD.Name, err)
			}
		}

		opts.Log.Info("Volume attached")
	}

	if !isAttached(state) {
		// Wait until attached to the KVVMI to update Status.Target.
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	}

	if r.setVMHotpluggedFinalizer(state) {
		err := opts.Client.Update(ctx, state.VMD)
		if err != nil {
			return fmt.Errorf("failed to set VMD finalizer with hotplugged block device %s: %w", state.VMD.Name, err)
		}
	}

	if r.setVMStatusBlockDeviceRefs(blockDeviceIndex, state) {
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

	state.VMBDA.Changed().Status.FailureReason = state.FailureReason
	state.VMBDA.Changed().Status.FailureMessage = state.FailureMessage

	if state.KVVMI == nil || state.VMD == nil {
		state.VMBDA.Changed().Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
		return nil
	}

	for _, volumeStatus := range state.KVVMI.Status.VolumeStatus {
		if volumeStatus.Name != kvbuilder.GenerateVMDDiskName(state.VMD.Name) {
			continue
		}

		switch volumeStatus.Phase {
		case virtv1.VolumeReady:
			state.VMBDA.Changed().Status.Phase = virtv2.BlockDeviceAttachmentPhaseAttached
		default:
			state.VMBDA.Changed().Status.Phase = virtv2.BlockDeviceAttachmentPhaseInProgress
		}

		break
	}

	return nil
}

func isAttached(state *VMBDAReconcilerState) bool {
	if state.KVVMI == nil || state.VMD == nil {
		return false
	}

	for _, status := range state.KVVMI.Status.VolumeStatus {
		if status.Name == kvbuilder.GenerateVMDDiskName(state.VMD.Name) {
			return status.Phase == virtv1.VolumeReady
		}
	}

	return false
}

// hotplugVolume requests kubevirt subresources APIService to attach volume to KVVMI.
func (r *VMBDAReconciler) hotplugVolume(ctx context.Context, state *VMBDAReconcilerState) error {
	if state.VMBDA.Current().Spec.BlockDeviceRef.Kind != virtv2.VMBDAObjectRefKindVirtualDisk {
		return fmt.Errorf("unknown block device attachment kind %s", state.VMBDA.Current().Spec.BlockDeviceRef.Kind)
	}

	name := kvbuilder.GenerateVMDDiskName(state.VMBDA.Current().Spec.BlockDeviceRef.Name)
	hotplugRequest := virtv1.AddVolumeOptions{
		Name: name,
		Disk: &virtv1.Disk{
			Name: name,
			DiskDevice: virtv1.DiskDevice{
				Disk: &virtv1.DiskTarget{
					Bus: "scsi",
				},
			},
			Serial: state.VMBDA.Current().Spec.BlockDeviceRef.Name,
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
	kv, err := kubevirt.New(ctx, state.Client, r.controllerNamespace)
	if err != nil {
		return err
	}
	kvApi := kvapi.New(state.Client, kv)
	err = kvApi.AddVolume(ctx, state.VMBDA.Current().Namespace, state.VMBDA.Current().Spec.VirtualMachine, &hotplugRequest)
	if err != nil {
		return fmt.Errorf("error adding volume, %w", err)
	}

	return nil
}

// unplugVolume requests kubevirt subresources APIService to detach volume from KVVMI.
func (r *VMBDAReconciler) unplugVolume(ctx context.Context, state *VMBDAReconcilerState) error {
	if state.VMBDA.Current().Spec.BlockDeviceRef.Kind != virtv2.VMBDAObjectRefKindVirtualDisk {
		return fmt.Errorf("unknown block device attachment type %s", state.VMBDA.Current().Spec.BlockDeviceRef.Kind)
	}

	name := kvbuilder.GenerateVMDDiskName(state.VMBDA.Current().Spec.BlockDeviceRef.Name)
	unplugRequest := virtv1.RemoveVolumeOptions{
		Name: name,
	}

	kv, err := kubevirt.New(ctx, state.Client, r.controllerNamespace)
	if err != nil {
		return err
	}
	kvApi := kvapi.New(state.Client, kv)
	err = kvApi.RemoveVolume(ctx, state.VMBDA.Current().Namespace, state.VMBDA.Current().Spec.VirtualMachine, &unplugRequest)
	if err != nil {
		return fmt.Errorf("error removing volume, %w", err)
	}

	return nil
}

func (r *VMBDAReconciler) setVMHotpluggedFinalizer(state *VMBDAReconcilerState) bool {
	return controllerutil.AddFinalizer(state.VMD, virtv2.FinalizerVMDProtection)
}

// setVMStatusBlockDeviceRefs copy volume status from KVVMI for attached disk to the d8 VM block devices status.
func (r *VMBDAReconciler) setVMStatusBlockDeviceRefs(blockDeviceIndex int, state *VMBDAReconcilerState) bool {
	var vs virtv1.VolumeStatus

	for i := range state.KVVMI.Status.VolumeStatus {
		if state.KVVMI.Status.VolumeStatus[i].Name == state.VMD.Name {
			vs = state.KVVMI.Status.VolumeStatus[i]
		}
	}

	if blockDeviceIndex > -1 {
		blockDevice := state.VM.Status.BlockDeviceRefs[blockDeviceIndex]
		if blockDevice.Target != vs.Target || blockDevice.Size != state.VMD.Status.Capacity {
			blockDevice.Target = vs.Target
			blockDevice.Size = state.VMD.Status.Capacity

			state.VM.Status.BlockDeviceRefs[blockDeviceIndex] = blockDevice

			return true
		}

		return false
	}

	state.VM.Status.BlockDeviceRefs = append(state.VM.Status.BlockDeviceRefs, virtv2.BlockDeviceStatusRef{
		Kind:         virtv2.DiskDevice,
		Name:         state.VMD.Name,
		Target:       vs.Target,
		Size:         state.VMD.Status.Capacity,
		Hotpluggable: true,
	})

	return true
}

// checkHotplugSanity detects if it is possible to hotplug disk to the VM.
// 1. It searches for disk in VM spec and returns false if disk is already attached to VM.
// 2. It returns false if VM is in the "Manual approve" mode.
func (r *VMBDAReconciler) checkHotplugSanity(state *VMBDAReconcilerState) (string, bool) {
	if state.VM == nil {
		return "", true
	}

	var messages []string

	// Check if disk is already in the spec of VM.
	diskName := state.VMBDA.Current().Spec.BlockDeviceRef.Name

	for _, bd := range state.VM.Spec.BlockDeviceRefs {
		if bd.Kind == virtv2.DiskDevice && bd.Name == diskName {
			messages = append(messages, fmt.Sprintf("disk %s is already attached to virtual machine", diskName))
			break
		}
	}
	if len(state.VM.Status.RestartAwaitingChanges) > 0 {
		messages = append(messages, "virtual machine waits for restart approval")
	}

	if len(messages) == 0 {
		return "", true
	}

	return strings.Join(messages, ", "), false
}
