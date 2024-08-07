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

package vmop

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Reconciler struct{}

func NewReconciler() *Reconciler {
	return &Reconciler{}
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineOperation{}), &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("error setting watch on VMOP: %w", err)
	}
	// Subscribe on VirtualMachines.
	if err = ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, vm client.Object) []reconcile.Request {
			c := mgr.GetClient()
			vmops := &virtv2.VirtualMachineOperationList{}
			if err := c.List(ctx, vmops, client.InNamespace(vm.GetNamespace())); err != nil {
				return nil
			}
			var requests []reconcile.Request
			for _, vmop := range vmops.Items {
				if vmop.Spec.VirtualMachine == vm.GetName() && vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vmop.GetNamespace(),
							Name:      vmop.GetName(),
						},
					})
					break
				}
			}
			return requests
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVM := e.ObjectOld.(*virtv2.VirtualMachine)
				newVM := e.ObjectNew.(*virtv2.VirtualMachine)
				return oldVM.Status.Phase != newVM.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}

func (r *Reconciler) Sync(ctx context.Context, req reconcile.Request, state *ReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	log := opts.Log.With("vmop.name", state.VMOP.Current().GetName())
	switch {
	case state.IsDeletion():
		log.Debug("Delete VMOP, remove protective finalizers")
		return r.removeFinalizers(ctx, state, opts)
	case state.IsCompleted():
		log.Debug("VMOP completed", "namespacedName", req.String())
		return r.removeFinalizers(ctx, state, opts)

	case state.IsFailed():
		log.Debug("VMOP failed", "namespacedName", req.String())
		return r.removeFinalizers(ctx, state, opts)
	case !state.IsProtected():
		// Set protective finalizer atomically.
		if controllerutil.AddFinalizer(state.VMOP.Changed(), virtv2.FinalizerVMOPCleanup) {
			state.SetReconcilerResult(&reconcile.Result{Requeue: true})
			return nil
		}
	case state.VmIsEmpty():
		return nil
	}
	found, err := state.OtherVMOPInProgress(ctx)
	if err != nil {
		return err
	}
	if found {
		state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 15 * time.Second})
		return nil
	}
	if !state.IsInProgress() {
		err = r.ensureVMFinalizers(ctx, state, opts)
		if err != nil {
			return err
		}
		if !r.isOperationAllowed(state.VMOP.Current().Spec.Type, state) {
			return nil
		}
		err = r.doOperation(ctx, state.VMOP.Current().Spec, state)
		if err != nil {
			msg := "The operation completed with an error."
			state.SetOperationResult(false, fmt.Sprintf("%s %s", msg, err.Error()))
			opts.Recorder.Event(state.VMOP.Current(), corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, msg)
			log.Error(msg, "err", err, "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())
			return nil
		}
		state.SetOperationResult(true, "")
		msg := "The operation completed without errors."
		opts.Recorder.Event(state.VMOP.Current(), corev1.EventTypeNormal, virtv2.ReasonVMOPSucceeded, msg)
		log.Debug(msg, "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())
		return nil
	}
	if r.IsCompleted(state.VMOP.Current().Spec.Type, state.VM.Status.Phase) {
		return nil
	}
	state.SetReconcilerResult(&reconcile.Result{RequeueAfter: 60 * time.Second})
	return nil
}

func (r *Reconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *ReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	log := opts.Log.With("vmop.name", state.VMOP.Current().GetName())
	log.Debug("Update VMOP status", "vmop.name", state.VMOP.Current().GetName(), "vmop.namespace", state.VMOP.Current().GetNamespace())

	if state.IsDeletion() {
		return nil
	}

	vmopStatus := state.VMOP.Current().Status.DeepCopy()

	switch {
	case state.IsFailed(), state.IsCompleted(), state.IsInProgress():
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
	}

	if result := state.GetOperationResult(); result != nil {
		if result.WasSuccessful() {
			vmopStatus.Phase = virtv2.VMOPPhaseInProgress
		} else {
			vmopStatus.Phase = virtv2.VMOPPhaseFailed
			vmopStatus.FailureReason = virtv2.ReasonErrVMOPFailed
			vmopStatus.FailureMessage = result.Message()
		}
	}
	if !state.VmIsEmpty() && state.IsInProgress() && r.IsCompleted(state.VMOP.Current().Spec.Type, state.VM.Status.Phase) {
		vmopStatus.Phase = virtv2.VMOPPhaseCompleted
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

func (r *Reconciler) removeFinalizers(ctx context.Context, state *ReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
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
	case virtv2.ManualPolicy, virtv2.AlwaysOnUnlessStoppedManually:
		return true
	default:
		return false
	}
}

func (r *Reconciler) isOperationAllowedForVmPhase(op virtv2.VMOPOperation, phase virtv2.MachinePhase) bool {
	if phase == virtv2.MachineTerminating ||
		phase == virtv2.MachinePending ||
		phase == virtv2.MachineMigrating {
		return false
	}
	switch op {
	case virtv2.VMOPOperationTypeStart:
		return phase == virtv2.MachineStopped || phase == virtv2.MachineStopping
	case virtv2.VMOPOperationTypeStop, virtv2.VMOPOperationTypeRestart:
		return phase == virtv2.MachineRunning ||
			phase == virtv2.MachineStarting ||
			phase == virtv2.MachinePause
	default:
		return false
	}
}

func (r *Reconciler) IsCompleted(op virtv2.VMOPOperation, phase virtv2.MachinePhase) bool {
	switch op {
	case virtv2.VMOPOperationTypeRestart, virtv2.VMOPOperationTypeStart:
		return phase == virtv2.MachineRunning
	case virtv2.VMOPOperationTypeStop:
		return phase == virtv2.MachineStopped
	default:
		return false
	}
}
