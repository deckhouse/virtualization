package vmop

import (
	"context"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

type VMOPReconciler struct {
}

func NewVMOPReconciler() *VMOPReconciler {
	return &VMOPReconciler{}
}

func (r *VMOPReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineOperation{}), &handler.EnqueueRequestForObject{})
}

func (r *VMOPReconciler) Sync(ctx context.Context, req reconcile.Request, state *VMOPReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	switch {
	case state.IsDeletion():
		opts.Log.V(1).Info("Delete VMOP, remove protective finalizers")
		return r.cleanupOnDeletion(ctx, state, opts)
	case !state.IsProtected():
		// Set protective finalizer atomically.
		if controllerutil.AddFinalizer(state.VMOP.Changed(), virtv2.FinalizerVMOPCleanup) {
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		}
	case state.IsCompleted():
		opts.Log.V(2).Info("VMOP completed", "namespacedName", req.String())
		return r.removeVMFinalizers(ctx, state, opts)

	case state.IsFailed():
		opts.Log.V(2).Info("VMOP failed", "namespacedName", req.String())
		return r.removeVMFinalizers(ctx, state, opts)
	case state.VmIsEmpty():
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 2 * time.Second})
		return nil
	}
	found, err := state.OtherVMOPInProgress(ctx)
	if err != nil {
		return err
	}
	if found {
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	}
	if !state.IsInProgress() {
		state.SetInProgress()
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return r.ensureVMFinalizers(ctx, state, opts)
	}

	if !r.isOperationAllowed(state.VMOP.Current().Spec.Type, state) {
		return nil
	}
	err = r.doOperation(ctx, state.VMOP.Current().Spec, state)
	if err != nil {
		msg := "The operation completed with an error."
		state.SetOperationResult(false, fmt.Sprintf("%s %s", msg, err.Error()))
		opts.Recorder.Event(state.VMOP.Current(), corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, msg)
		opts.Log.V(1).Error(err, "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())
	} else {
		state.SetOperationResult(true, "")
		msg := "The operation completed without errors."
		opts.Recorder.Event(state.VMOP.Current(), corev1.EventTypeNormal, virtv2.ReasonVMOPSucceeded, msg)
		opts.Log.V(2).Info(msg, "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())
	}
	return nil
}

func (r *VMOPReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *VMOPReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.V(2).Info("Update VMOP status", "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())

	if state.IsDeletion() {
		return nil
	}

	vmopStatus := state.VMOP.Current().Status.DeepCopy()

	switch {
	case state.IsFailed(), state.IsCompleted():
		// No need to update status.
		break
	case vmopStatus.Phase == "":
		vmopStatus.Phase = virtv2.VMOPPhasePending
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
	case state.VmIsEmpty():
		vmopStatus.Phase = virtv2.VMOPPhasePending
	case !r.isOperationAllowedForRunPolicy(state.VMOP.Current().Spec.Type, state.VM.Spec.RunPolicy):
		vmopStatus.Phase = virtv2.VMOPPhaseFailed
		vmopStatus.FailureReason = virtv2.ReasonErrVMOPNotPermitted
		vmopStatus.FailureMessage = fmt.Sprintf("operation %q not permitted for vm.spec.runPolicy=%q", state.VMOP.Current().Spec.Type, state.VM.Spec.RunPolicy)
	case !r.isOperationAllowedForVmPhase(state.VMOP.Current().Spec.Type, state.VM.Status.Phase):
		vmopStatus.Phase = virtv2.VMOPPhaseFailed
		vmopStatus.FailureReason = virtv2.ReasonErrVMOPNotPermitted
		vmopStatus.FailureMessage = fmt.Sprintf("operation %q not permitted for vm.status.phase=%q", state.VMOP.Current().Spec.Type, state.VM.Status.Phase)
	case state.GetInProgress():
		vmopStatus.Phase = virtv2.VMOPPhaseInProgress
	}

	if result := state.GetOperationResult(); result != nil {
		if result.WasSuccessful() {
			vmopStatus.Phase = virtv2.VMOPPhaseCompleted
		} else {
			vmopStatus.Phase = virtv2.VMOPPhaseFailed
			vmopStatus.FailureReason = virtv2.ReasonErrVMOPFailed
			vmopStatus.FailureMessage = result.Message()
		}
	}
	state.VMOP.Changed().Status = *vmopStatus
	return nil
}

func (r *VMOPReconciler) IsProtected(obj client.Object) bool {
	return controllerutil.ContainsFinalizer(obj, virtv2.FinalizerVMOPProtection)
}

func (r *VMOPReconciler) ensureVMFinalizers(ctx context.Context, state *VMOPReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.VM != nil && controllerutil.AddFinalizer(state.VM, virtv2.FinalizerVMOPProtection) {
		if err := opts.Client.Update(ctx, state.VM); err != nil {
			return fmt.Errorf("error setting finalizer on a VM %q: %w", state.VM.Name, err)
		}
	}
	return nil
}

func (r *VMOPReconciler) removeVMFinalizers(ctx context.Context, state *VMOPReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.VM != nil && controllerutil.RemoveFinalizer(state.VM, virtv2.FinalizerVMOPProtection) {
		if err := opts.Client.Update(ctx, state.VM); err != nil {
			return fmt.Errorf("unable to remove VM %q finalizer %q: %w", state.VM.Name, virtv2.FinalizerVMOPProtection, err)
		}
	}
	return nil
}

func (r *VMOPReconciler) cleanupOnDeletion(ctx context.Context, state *VMOPReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if err := r.removeVMFinalizers(ctx, state, opts); err != nil {
		return err
	}
	controllerutil.RemoveFinalizer(state.VMOP.Changed(), virtv2.FinalizerVMOPCleanup)
	return nil
}

func (r *VMOPReconciler) doOperation(ctx context.Context, operationSpec virtv2.VirtualMachineOperationSpec, state *VMOPReconcilerState) error {
	switch operationSpec.Type {
	case virtv2.VMOPOperationTypeStart:
		return r.doOperationStart(ctx, state)
	case virtv2.VMOPOperationTypeStop:
		return r.doOperationStop(ctx, operationSpec.Force, state)
	case virtv2.VMOPOperationTypeRestart:
		return r.doOperationRestart(ctx, operationSpec.Force, state)
	default:
		return fmt.Errorf("unexpected opearation %q. %w", operationSpec.Type, common.ErrUnknownValue)
	}
}

func (r *VMOPReconciler) doOperationStart(ctx context.Context, state *VMOPReconcilerState) error {
	//kvvm, err := state.GetKVVM(ctx)
	//if err != nil {
	//	return fmt.Errorf("cannot get kvvm %q. %w", state.VM.Name, err)
	//}
	//request := &virtv1.VirtualMachineStateChangeRequest{}

	return nil
}

func (r *VMOPReconciler) getChecngeRequest(vm *virtv1.VirtualMachine, changes ...virtv1.VirtualMachineStateChangeRequest) {
	//var ops []string
	//
	//verb := "add"
	//// Special case: if there's no status field at all, add one.
	//newStatus := virtv1.VirtualMachineStatus{}
	//if equality.Semantic.DeepEqual(vm.Status, newStatus) {
	//	for _, change := range changes {
	//		newStatus.StateChangeRequests = append(newStatus.StateChangeRequests, change)
	//	}
	//}
}

func (r *VMOPReconciler) doOperationStop(ctx context.Context, force bool, state *VMOPReconcilerState) error {
	kvvmi, err := state.GetKVVMI(ctx)
	if err != nil {
		return fmt.Errorf("cannot get kvvmi %q. %w", state.VM.Name, err)
	}
	if force {
		return state.Client.Delete(ctx, kvvmi)
	}
	// TODO: add soft stop
	return state.Client.Delete(ctx, kvvmi)
}

func (r *VMOPReconciler) doOperationRestart(ctx context.Context, force bool, state *VMOPReconcilerState) error {
	// TODO softreboot virt-handler
	kvvmi, err := state.GetKVVM(ctx)
	if err != nil {
		return fmt.Errorf("cannot get kvvmi %q. %w", state.VM.Name, err)
	}
	return state.Client.Delete(ctx, kvvmi)
}

func (r *VMOPReconciler) isOperationAllowed(op string, state *VMOPReconcilerState) bool {
	if state.VmIsEmpty() {
		return false
	}
	return r.isOperationAllowedForRunPolicy(op, state.VM.Spec.RunPolicy) && r.isOperationAllowedForVmPhase(op, state.VM.Status.Phase)
}

func (r *VMOPReconciler) isOperationAllowedForRunPolicy(op string, runPolicy virtv2.RunPolicy) bool {
	switch runPolicy {
	case virtv2.AlwaysOnPolicy:
		return op == virtv2.VMOPOperationTypeRestart
	case virtv2.AlwaysOffPolicy:
		return false
	case virtv2.ManualPolicy, virtv2.AlwaysOnUnlessStoppedManualy:
		return true
	default:
		return false
	}
}

func (r *VMOPReconciler) isOperationAllowedForVmPhase(op string, phase virtv2.MachinePhase) bool {
	if phase == virtv2.MachineTerminating ||
		phase == virtv2.MachinePending ||
		phase == virtv2.MachineScheduling ||
		phase == virtv2.MachineMigrating {
		return false
	}
	switch op {
	case virtv2.VMOPOperationTypeStart:
		return phase == virtv2.MachineStopped || phase == virtv2.MachineStopping || phase == virtv2.MachineFailed
	case virtv2.VMOPOperationTypeStop, virtv2.VMOPOperationTypeRestart:
		return phase == virtv2.MachineRunning ||
			phase == virtv2.MachineFailed ||
			phase == virtv2.MachineStarting ||
			phase == virtv2.MachinePause
	default:
		return false
	}
}
