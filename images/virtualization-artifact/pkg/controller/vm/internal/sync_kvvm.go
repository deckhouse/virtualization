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
	"errors"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	vmutil "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameSyncKvvmHandler = "SyncKvvmHandler"

func NewSyncKvvmHandler(dvcrSettings *dvcr.Settings, client client.Client, recorder record.EventRecorder) *SyncKvvmHandler {
	return &SyncKvvmHandler{
		dvcrSettings: dvcrSettings,
		client:       client,
		recorder:     recorder,
	}
}

type SyncKvvmHandler struct {
	client       client.Client
	recorder     record.EventRecorder
	dvcrSettings *dvcr.Settings
}

func (h *SyncKvvmHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log, ctx := logger.GetHandlerContext(ctx, nameSyncKvvmHandler)

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	cbConfApplied := conditions.NewConditionBuilder(vmcondition.TypeConfigurationApplied).
		Generation(current.GetGeneration()).
		Status(metav1.ConditionUnknown).
		Reason(conditions.ReasonUnknown)

	cbAwaitingRestart := conditions.NewConditionBuilder(vmcondition.TypeAwaitingRestartToApplyConfiguration).
		Generation(current.GetGeneration()).
		Status(metav1.ConditionFalse).
		Reason(vmcondition.ReasonRestartNoNeed)

	defer func() {
		conditions.SetCondition(cbConfApplied, &changed.Status.Conditions)
		conditions.SetCondition(cbAwaitingRestart, &changed.Status.Conditions)
	}()

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, err
	}
	class, err := s.Class(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 1. Set RestartAwaitingChanges.
	var lastAppliedSpec *virtv2.VirtualMachineSpec
	var changes vmchange.SpecChanges
	if kvvm != nil {
		lastAppliedSpec = h.loadLastAppliedSpec(current, kvvm)
		lastClassAppliedSpec := h.loadClassLastAppliedSpec(class, kvvm)
		changes = h.detectSpecChanges(ctx, kvvm, &current.Spec, lastAppliedSpec, &class.Spec, lastClassAppliedSpec)
	}

	if kvvm == nil || changes.IsEmpty() {
		changed.Status.RestartAwaitingChanges = nil
	} else {
		changed.Status.RestartAwaitingChanges, err = changes.ConvertPendingChanges()
		if err != nil {
			err = fmt.Errorf("failed to generate pending configuration changes: %w", err)
			cbConfApplied.
				Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonConfigurationNotApplied).
				Message(service.CapitalizeFirstLetter(err.Error()) + ".")
			return reconcile.Result{}, err
		}
	}

	// 2. Wait if dependent resources are not ready yet.
	if h.isWaiting(changed) {
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message(
				"Waiting for the dependent resources. Be careful restarting the virtual machine: " +
					"the virtual machine cannot be restarted immediately to apply pending configuration changes " +
					"as it is awaiting the availability of dependent resources.",
			)
		return reconcile.Result{RequeueAfter: time.Minute}, nil
	}

	var errs error

	// 3. Create or update KVVM.
	synced, kvvmSyncErr := h.syncKVVM(ctx, s, changes)
	if kvvmSyncErr != nil {
		errs = errors.Join(errs, fmt.Errorf("failed to sync the internal virtual machine: %w", kvvmSyncErr))
	}

	if synced {
		// 3.1. Changes are applied, consider current spec as last applied.
		changed.Status.RestartAwaitingChanges = nil
	}

	// 4. Set ConfigurationApplied condition.
	switch {
	case errs != nil:
		h.recorder.Event(current, corev1.EventTypeWarning, virtv2.ReasonErrVmNotSynced, kvvmSyncErr.Error())
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message(service.CapitalizeFirstLetter(errs.Error()) + ".")
	case len(changed.Status.RestartAwaitingChanges) > 0:
		h.recorder.Event(current, corev1.EventTypeNormal, virtv2.ReasonErrRestartAwaitingChanges, "The virtual machine configuration successfully synced")
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message("Waiting for the user to restart in order to apply the configuration changes.")
		cbAwaitingRestart.
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonRestartAwaitingChangesExist).
			Message("Waiting for the user to restart in order to apply the configuration changes.")
	case synced:
		h.recorder.Event(current, corev1.EventTypeNormal, virtv2.ReasonErrVmSynced, "The virtual machine configuration successfully synced")
		cbConfApplied.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonConfigurationApplied)
	default:
		log.Error("Unexpected case during kvvm sync, please report a bug")
	}

	return reconcile.Result{}, errs
}

func (h *SyncKvvmHandler) Name() string {
	return nameSyncKvvmHandler
}

func (h *SyncKvvmHandler) isWaiting(vm *virtv2.VirtualMachine) bool {
	for _, c := range vm.Status.Conditions {
		switch vmcondition.Type(c.Type) {
		case vmcondition.TypeBlockDevicesReady:
			if c.Status != metav1.ConditionTrue && c.Reason != vmcondition.ReasonWaitingForProvisioningToPVC.String() {
				return true
			}

		case vmcondition.TypeSnapshotting:
			if c.Status == metav1.ConditionTrue && c.Reason == vmcondition.ReasonSnapshottingInProgress.String() {
				return true
			}

		case vmcondition.TypeIPAddressReady:
			if c.Status != metav1.ConditionTrue && c.Reason != vmcondition.ReasonIPAddressNotAssigned.String() {
				return true
			}

		case vmcondition.TypeProvisioningReady,
			vmcondition.TypeClassReady:
			if c.Status != metav1.ConditionTrue {
				return true
			}

		case vmcondition.TypeDiskAttachmentCapacityAvailable:
			if c.Status != metav1.ConditionTrue {
				return true
			}

		case vmcondition.TypeSizingPolicyMatched:
			if c.Status != metav1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func (h *SyncKvvmHandler) syncKVVM(ctx context.Context, s state.VirtualMachineState, changes vmchange.SpecChanges) (bool, error) {
	if s.VirtualMachine().IsEmpty() {
		return false, fmt.Errorf("the virtual machine is empty, please report a bug")
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return false, fmt.Errorf("find the internal virtual machine: %w", err)
	}

	if kvvm == nil {
		err = h.createKVVM(ctx, s)
		if err != nil {
			return false, fmt.Errorf("create the internal virtual machine: %w", err)
		}

		return true, nil
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return false, fmt.Errorf("find the internal virtual machine instance: %w", err)
	}
	pod, err := s.Pod(ctx)
	if err != nil {
		return false, fmt.Errorf("find the virtual machine pod: %w", err)
	}

	switch {
	case h.canApplyChanges(s.VirtualMachine().Current(), kvvm, kvvmi, pod, changes):
		// No need to wait, apply changes to KVVM immediately.
		err = h.applyVMChangesToKVVM(ctx, s, changes)
		if err != nil {
			return false, fmt.Errorf("apply changes to the internal virtual machine: %w", err)
		}

		return true, nil
	case changes.IsEmpty():
		return true, nil
	default:
		// Delay changes propagation to KVVM until user restarts VM.
		return false, nil
	}
}

// createKVVM constructs and creates new KubeVirt VirtualMachine based on d8 VirtualMachine spec.
func (h *SyncKvvmHandler) createKVVM(ctx context.Context, s state.VirtualMachineState) error {
	log := logger.FromContext(ctx)

	if s.VirtualMachine().IsEmpty() {
		return fmt.Errorf("the virtual machine is empty, please report a bug")
	}
	kvvm, err := h.makeKVVMFromVMSpec(ctx, s)
	if err != nil {
		return fmt.Errorf("failed to make the internal virtual machine: %w", err)
	}

	if err = h.client.Create(ctx, kvvm); err != nil {
		return fmt.Errorf("failed to create the internal virtual machine: %w", err)
	}

	log.Info("Created new KubeVirt VM", "name", kvvm.Name)
	log.Debug("Created new KubeVirt VM", "name", kvvm.Name, "kvvm", kvvm)

	return nil
}

// updateKVVM constructs and creates new KubeVirt VirtualMachine based on d8 VirtualMachine spec.
func (h *SyncKvvmHandler) updateKVVM(ctx context.Context, s state.VirtualMachineState) error {
	log := logger.FromContext(ctx)

	if s.VirtualMachine().IsEmpty() {
		return fmt.Errorf("the virtual machine is empty, please report a bug")
	}

	kvvm, err := h.makeKVVMFromVMSpec(ctx, s)
	if err != nil {
		return fmt.Errorf("failed to prepare the internal virtual machine: %w", err)
	}

	if err = h.client.Update(ctx, kvvm); err != nil {
		return fmt.Errorf("failed to create the internal virtual machine: %w", err)
	}

	log.Info("Update KubeVirt VM done", "name", kvvm.Name)
	log.Debug("Update KubeVirt VM done", "name", kvvm.Name, "kvvm", kvvm)

	return nil
}

// restartKVVM deletes KVVMI to restart VM.
func (h *SyncKvvmHandler) restartKVVM(ctx context.Context, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) error {
	err := powerstate.RestartVM(ctx, h.client, kvvm, kvvmi, false)
	if err != nil {
		return fmt.Errorf("failed to restart the current internal virtual machine instance: %w", err)
	}

	return nil
}

func (h *SyncKvvmHandler) makeKVVMFromVMSpec(ctx context.Context, s state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
	if s.VirtualMachine().IsEmpty() {
		return nil, nil
	}
	current := s.VirtualMachine().Current()
	kvvmName := object.NamespacedName(current)

	kvvmOpts := kvbuilder.KVVMOptions{
		EnableParavirtualization: current.Spec.EnableParavirtualization,
		OsType:                   current.Spec.OsType,
		DisableHypervSyNIC:       os.Getenv("DISABLE_HYPERV_SYNIC") == "1",
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return nil, err
	}
	var kvvmBuilder *kvbuilder.KVVM
	if kvvm == nil {
		kvvmBuilder = kvbuilder.NewEmptyKVVM(kvvmName, kvvmOpts)
	} else {
		kvvmBuilder = kvbuilder.NewKVVM(kvvm.DeepCopy(), kvvmOpts)
	}
	bdState := NewBlockDeviceState(s)
	err = bdState.Reload(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to relaod blockdevice state for the virtual machine: %w", err)
	}
	class, err := s.Class(ctx)
	if err != nil {
		return nil, err
	}
	ip, err := s.IPAddress(ctx)
	if err != nil {
		return nil, err
	}

	if ip.Status.Address == "" {
		return nil, fmt.Errorf("the IP address is not found for the virtual machine")
	}

	// Create kubevirt VirtualMachine resource from d8 VirtualMachine spec.
	err = kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, current, bdState.VDByName, bdState.VIByName, bdState.CVIByName, class, ip.Status.Address)
	if err != nil {
		return nil, err
	}
	newKVVM := kvvmBuilder.GetResource()

	err = kvbuilder.SetLastAppliedSpec(newKVVM, current)
	if err != nil {
		return nil, fmt.Errorf("set vm last applied spec on the internal virtual machine: %w", err)
	}

	err = kvbuilder.SetLastAppliedClassSpec(newKVVM, class)
	if err != nil {
		return nil, fmt.Errorf("set vmclass last applied spec on the internal virtual machine: %w", err)
	}

	return newKVVM, nil
}

func (h *SyncKvvmHandler) loadLastAppliedSpec(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) *virtv2.VirtualMachineSpec {
	if kvvm == nil || vm == nil {
		return nil
	}

	lastSpec, err := kvbuilder.LoadLastAppliedSpec(kvvm)
	// TODO Add smarter handler for empty/invalid annotation.
	if lastSpec == nil && err == nil {
		h.recorder.Event(vm, corev1.EventTypeWarning, virtv2.ReasonVMLastAppliedSpecInvalid, "Could not find last applied spec. Possible old VM or partial backup restore. Restart or recreate VM to adopt it.")
		lastSpec = &virtv2.VirtualMachineSpec{}
	}
	if err != nil {
		msg := fmt.Sprintf("Could not restore last applied spec: %v. Possible old VM or partial backup restore. Restart or recreate VM to adopt it.", err)
		h.recorder.Event(vm, corev1.EventTypeWarning, virtv2.ReasonVMLastAppliedSpecInvalid, msg)
		// In Automatic mode changes are applied immediately, so last-applied-spec annotation will be restored.
		if vmutil.ApprovalMode(vm) == virtv2.Automatic {
			lastSpec = &virtv2.VirtualMachineSpec{}
		}
		if vmutil.ApprovalMode(vm) == virtv2.Manual {
			// Manual mode requires meaningful content in status.pendingChanges.
			// There are different paths:
			//   1. Return err and do nothing, user should restore annotation or recreate VM.
			//   2. Use empty VirtualMachineSpec and show full replace in status.pendingChanges.
			//      This may lead to unexpected restart.
			//   3. Restore some fields from KVVM spec to prevent unexpected restarts and reduce
			//      content in status.pendingChanges.
			//
			// At this time, variant 2 is chosen.
			// TODO(future): Implement variant 3: restore some fields from KVVM.
			lastSpec = &virtv2.VirtualMachineSpec{}
		}
	}

	return lastSpec
}

func (h *SyncKvvmHandler) loadClassLastAppliedSpec(class *virtv2.VirtualMachineClass, kvvm *virtv1.VirtualMachine) *virtv2.VirtualMachineClassSpec {
	if kvvm == nil || class == nil {
		return nil
	}

	lastSpec, err := kvbuilder.LoadLastAppliedClassSpec(kvvm)
	// TODO Add smarter handler for empty/invalid annotation.
	if lastSpec == nil && err == nil {
		h.recorder.Event(class, corev1.EventTypeWarning, virtv2.ReasonVMClassLastAppliedSpecInvalid, "Could not find last applied spec. Possible old VMClass or partial backup restore. Restart or recreate VM to adopt it.")
		lastSpec = &virtv2.VirtualMachineClassSpec{}
	}
	if err != nil {
		msg := fmt.Sprintf("Could not restore last applied spec: %v. Possible old VMClass or partial backup restore. Restart or recreate VM to adopt it.", err)
		h.recorder.Event(class, corev1.EventTypeWarning, virtv2.ReasonVMClassLastAppliedSpecInvalid, msg)
		lastSpec = &virtv2.VirtualMachineClassSpec{}
	}

	return lastSpec
}

// detectSpecChanges compares KVVM generated from current VM spec with in cluster KVVM
// to calculate changes and action needed to apply these changes.
func (h *SyncKvvmHandler) detectSpecChanges(
	ctx context.Context,
	kvvm *virtv1.VirtualMachine,
	currentSpec, lastSpec *virtv2.VirtualMachineSpec,
	currentClassSpec, lastClassSpec *virtv2.VirtualMachineClassSpec,
) vmchange.SpecChanges {
	log := logger.FromContext(ctx)

	// Not applicable if KVVM is absent.
	if kvvm == nil || lastSpec == nil {
		return vmchange.SpecChanges{}
	}

	// Compare VM spec applied to the underlying KVVM
	// with the current VM spec (maybe edited by the user).
	specChanges := vmchange.CompareSpecs(lastSpec, currentSpec, currentClassSpec, lastClassSpec)

	log.Info(fmt.Sprintf("detected changes: empty %v, disruptive %v, actionType %v", specChanges.IsEmpty(), specChanges.IsDisruptive(), specChanges.ActionType()))
	log.Info(fmt.Sprintf("detected changes JSON: %s", specChanges.ToJSON()))

	return specChanges
}

// canApplyChanges returns true if changes can be applied right now.
//
// Wait if changes are disruptive, and approval mode is manual, and VM is still running.
func (h *SyncKvvmHandler) canApplyChanges(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, pod *corev1.Pod, changes vmchange.SpecChanges) bool {
	if vm == nil || changes.IsEmpty() {
		return false
	}
	if vmutil.ApprovalMode(vm) == virtv2.Automatic || !changes.IsDisruptive() || kvvmi == nil {
		return true
	}

	if vmIsPending(kvvm) {
		return true
	}

	// VM is stopped if instance is not created or Pod is in the Complete state.
	podStopped := true
	if pod != nil {
		phase := pod.Status.Phase
		podStopped = phase != corev1.PodPending && phase != corev1.PodRunning
	}

	return vmIsStopped(kvvm) && (!vmIsCreated(kvvm) || podStopped)
}

// applyVMChangesToKVVM applies updates to underlying KVVM based on actions type.
func (h *SyncKvvmHandler) applyVMChangesToKVVM(ctx context.Context, s state.VirtualMachineState, changes vmchange.SpecChanges) error {
	log := logger.FromContext(ctx)

	if changes.IsEmpty() || s.VirtualMachine().IsEmpty() {
		return nil
	}
	current := s.VirtualMachine().Current()
	action := changes.ActionType()

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return err
	}
	if kvvmi == nil && action == vmchange.ActionRestart {
		action = vmchange.ActionApplyImmediate
	}
	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return err
	}

	switch action {
	case vmchange.ActionRestart:
		log.Info("Restart VM to apply changes", "vm.name", current.GetName())

		h.recorder.Event(current, corev1.EventTypeNormal, virtv2.ReasonVMChangesApplied, "Apply disruptive changes")
		h.recorder.Event(current, corev1.EventTypeNormal, virtv2.ReasonVMRestarted, "")

		// Update KVVM spec according the current VM spec.
		if err = h.updateKVVM(ctx, s); err != nil {
			return fmt.Errorf("failed to update the internal virtual machine using the new spec: %w", err)
		}
		// Ask kubevirt to re-create KVVMI to apply new spec from KVVM.
		if err = h.restartKVVM(ctx, kvvm, kvvmi); err != nil {
			return fmt.Errorf("failed to restart the internal virtual machine instance to apply changes: %w", err)
		}

	case vmchange.ActionApplyImmediate:
		message := "Apply changes without restart"
		if changes.IsDisruptive() {
			message = "Apply disruptive changes without restart"
		}
		log.Info(message, "vm.name", current.GetName(), "action", changes)
		h.recorder.Event(current, corev1.EventTypeNormal, virtv2.ReasonVMChangesApplied, message)

		if err := h.updateKVVM(ctx, s); err != nil {
			return fmt.Errorf("unable to update KVVM using new VM spec: %w", err)
		}

	case vmchange.ActionNone:
		log.Info("No changes to underlying KVVM, update last-applied-spec annotation", "vm.name", current.GetName())

		class, err := s.Class(ctx)
		if err != nil {
			return fmt.Errorf("failed to get vmclass: %w", err)
		}
		if err = h.updateKVVMLastAppliedSpec(ctx, current, kvvm, class); err != nil {
			return fmt.Errorf("unable to update last-applied-spec on KVVM: %w", err)
		}
	}
	return nil
}

// updateKVVMLastAppliedSpec updates last-applied-spec annotation on KubeVirt VirtualMachine.
func (h *SyncKvvmHandler) updateKVVMLastAppliedSpec(
	ctx context.Context,
	vm *virtv2.VirtualMachine,
	kvvm *virtv1.VirtualMachine,
	class *virtv2.VirtualMachineClass,
) error {
	if vm == nil || kvvm == nil {
		return nil
	}

	err := kvbuilder.SetLastAppliedSpec(kvvm, vm)
	if err != nil {
		return fmt.Errorf("set vm last applied spec on KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}
	err = kvbuilder.SetLastAppliedClassSpec(kvvm, class)
	if err != nil {
		return fmt.Errorf("set vmclass last applied spec on KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	if err := h.client.Update(ctx, kvvm); err != nil {
		return fmt.Errorf("unable to update KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	log := logger.FromContext(ctx)
	log.Info("Update last applied spec on KubeVirt VM done", "name", kvvm.GetName())

	return nil
}
