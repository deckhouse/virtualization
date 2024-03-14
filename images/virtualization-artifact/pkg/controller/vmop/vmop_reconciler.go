package vmop

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

type Reconciler struct{}

func NewReconciler() *Reconciler {
	return &Reconciler{}
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineOperation{}), &handler.EnqueueRequestForObject{})
}

func (r *Reconciler) Sync(ctx context.Context, req reconcile.Request, state *ReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	log := opts.Log.WithValues("vmop.name", state.VMOP.Current().GetName())

	switch {
	case state.IsDeletion():
		log.V(1).Info("Delete VMOP, remove protective finalizers")
		return r.cleanupOnDeletion(ctx, state, opts)
	case !state.IsProtected():
		// Set protective finalizer atomically.
		if controllerutil.AddFinalizer(state.VMOP.Changed(), virtv2.FinalizerVMOPCleanup) {
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		}
	case state.IsCompleted():
		log.V(2).Info("VMOP completed", "namespacedName", req.String())
		return r.removeVMFinalizers(ctx, state, opts)

	case state.IsFailed():
		log.V(2).Info("VMOP failed", "namespacedName", req.String())
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
		log.V(1).Error(err, msg, "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())
	} else {
		state.SetOperationResult(true, "")
		msg := "The operation completed without errors."
		opts.Recorder.Event(state.VMOP.Current(), corev1.EventTypeNormal, virtv2.ReasonVMOPSucceeded, msg)
		log.V(2).Info(msg, "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())
	}
	return nil
}

func (r *Reconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *ReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	log := opts.Log.WithValues("vmop.name", state.VMOP.Current().GetName())
	log.V(2).Info("Update VMOP status", "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())

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

func (r *Reconciler) IsProtected(obj client.Object) bool {
	return controllerutil.ContainsFinalizer(obj, virtv2.FinalizerVMOPProtection)
}

func (r *Reconciler) ensureVMFinalizers(ctx context.Context, state *ReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.VM != nil && controllerutil.AddFinalizer(state.VM, virtv2.FinalizerVMOPProtection) {
		if err := opts.Client.Update(ctx, state.VM); err != nil {
			return fmt.Errorf("error setting finalizer on a VM %q: %w", state.VM.Name, err)
		}
	}
	return nil
}

func (r *Reconciler) removeVMFinalizers(ctx context.Context, state *ReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if state.VM != nil && controllerutil.RemoveFinalizer(state.VM, virtv2.FinalizerVMOPProtection) {
		if err := opts.Client.Update(ctx, state.VM); err != nil {
			return fmt.Errorf("unable to remove VM %q finalizer %q: %w", state.VM.Name, virtv2.FinalizerVMOPProtection, err)
		}
	}
	return nil
}

func (r *Reconciler) cleanupOnDeletion(ctx context.Context, state *ReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	if err := r.removeVMFinalizers(ctx, state, opts); err != nil {
		return err
	}
	controllerutil.RemoveFinalizer(state.VMOP.Changed(), virtv2.FinalizerVMOPCleanup)
	return nil
}

func (r *Reconciler) doOperation(ctx context.Context, operationSpec virtv2.VirtualMachineOperationSpec, state *ReconcilerState) error {
	switch operationSpec.Type {
	case virtv2.VMOPOperationTypeStart:
		return r.doOperationStart(ctx, state)
	case virtv2.VMOPOperationTypeStop:
		return r.doOperationStop(ctx, operationSpec.Force, state)
	case virtv2.VMOPOperationTypeRestart:
		return r.doOperationRestart(ctx, operationSpec.Force, state)
	default:
		return fmt.Errorf("unexpected operation %q. %w", operationSpec.Type, common.ErrUnknownValue)
	}
}

func (r *Reconciler) doOperationStart(ctx context.Context, state *ReconcilerState) error {
	kvvm, err := state.GetKVVM(ctx)
	if err != nil {
		return fmt.Errorf("cannot get kvvm %q. %w", state.VM.Name, err)
	}
	return powerstate.StartVM(ctx, state.Client, kvvm)
}

func (r *Reconciler) doOperationStop(ctx context.Context, force bool, state *ReconcilerState) error {
	kvvmi, err := state.GetKVVMI(ctx)
	if err != nil {
		return fmt.Errorf("cannot get kvvmi %q. %w", state.VM.Name, err)
	}
	return powerstate.StopVM(ctx, state.Client, kvvmi, force)
}

func (r *Reconciler) doOperationRestart(ctx context.Context, force bool, state *ReconcilerState) error {
	kvvm, err := state.GetKVVM(ctx)
	if err != nil {
		return fmt.Errorf("cannot get kvvm %q. %w", state.VM.Name, err)
	}
	kvvmi, err := state.GetKVVMI(ctx)
	if err != nil {
		return fmt.Errorf("cannot get kvvmi %q. %w", state.VM.Name, err)
	}
	return powerstate.RestartVM(ctx, state.Client, kvvm, kvvmi, force)
}

func (r *Reconciler) isOperationAllowed(op virtv2.VMOPOperation, state *ReconcilerState) bool {
	if state.VmIsEmpty() {
		return false
	}
	return r.isOperationAllowedForRunPolicy(op, state.VM.Spec.RunPolicy) && r.isOperationAllowedForVmPhase(op, state.VM.Status.Phase)
}

func (r *Reconciler) isOperationAllowedForRunPolicy(op virtv2.VMOPOperation, runPolicy virtv2.RunPolicy) bool {
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

func (r *Reconciler) isOperationAllowedForVmPhase(op virtv2.VMOPOperation, phase virtv2.MachinePhase) bool {
	if phase == virtv2.MachineTerminating ||
		phase == virtv2.MachinePending ||
		phase == virtv2.MachineScheduling ||
		phase == virtv2.MachineMigrating {
		return false
	}
	switch op {
	case virtv2.VMOPOperationTypeStart:
		return phase == virtv2.MachineStopped || phase == virtv2.MachineStopping
	case virtv2.VMOPOperationTypeStop, virtv2.VMOPOperationTypeRestart:
		return phase == virtv2.MachineRunning ||
			phase == virtv2.MachineFailed ||
			phase == virtv2.MachineStarting ||
			phase == virtv2.MachinePause
	default:
		return false
	}
}
