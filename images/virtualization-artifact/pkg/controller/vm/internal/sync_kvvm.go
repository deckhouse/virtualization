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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	vmutil "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameSyncKvvmHandler = "SyncKvvmHandler"

var syncKVVMConditions = []string{
	string(vmcondition.TypeConfigurationApplied),
	string(vmcondition.TypeAwaitingRestartToApplyConfiguration),
}

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

	if update := addAllUnknown(changed, syncKVVMConditions...); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	mgr := conditions.NewManager(changed.Status.Conditions)

	if h.isWaiting(changed) {
		mgr.Update(conditions.NewConditionBuilder2(vmcondition.TypeConfigurationApplied).
			Generation(current.GetGeneration()).
			Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonConfigurationNotApplied).
			Message("Waiting for dependent resources. Afterwards, configuration may be applied.").
			Condition())
		mgr.Update(conditions.NewConditionBuilder2(vmcondition.TypeAwaitingRestartToApplyConfiguration).
			Generation(current.GetGeneration()).
			Message("Waiting for dependent resources.").
			Reason2(vmcondition.ReasonRestartNoNeed).
			Status(metav1.ConditionFalse).
			Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}
	if err := h.syncKVVM(ctx, s); err != nil {
		log.Error(fmt.Sprintf("Failed to sync kvvm: %v", err))
		h.recorder.Event(current, corev1.EventTypeWarning, virtv2.ReasonErrVmNotSynced, err.Error())
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
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
		case vmcondition.TypeIPAddressReady,
			vmcondition.TypeProvisioningReady,
			vmcondition.TypeClassReady:
			if c.Status != metav1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func (h *SyncKvvmHandler) syncKVVM(ctx context.Context, s state.VirtualMachineState) error {
	log := logger.FromContext(ctx)

	if s.VirtualMachine().IsEmpty() {
		return fmt.Errorf("VM is empty")
	}
	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return err
	}

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	mgr := conditions.NewManager(changed.Status.Conditions)

	if kvvm == nil {
		mgr.Update(conditions.NewConditionBuilder2(vmcondition.TypeAwaitingRestartToApplyConfiguration).
			Generation(current.GetGeneration()).
			Reason2(vmcondition.ReasonRestartNoNeed).
			Status(metav1.ConditionFalse).
			Condition())

		cb := conditions.NewConditionBuilder2(vmcondition.TypeConfigurationApplied).
			Generation(current.GetGeneration())
		err = h.createKVVM(ctx, s)
		if err != nil {
			cb.Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonConfigurationNotApplied).
				Message(fmt.Sprintf("Failed to apply configuration: %s", err.Error()))
		} else {
			cb.Status(metav1.ConditionTrue).
				Reason2(vmcondition.ReasonConfigurationApplied)
		}
		mgr.Update(cb.Condition())
		changed.Status.Conditions = mgr.Generate()
		return err
	}

	lastAppliedSpec := h.loadLastAppliedSpec(current, kvvm)
	changes := h.detectSpecChanges(ctx, kvvm, &current.Spec, lastAppliedSpec)

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return err
	}
	pod, err := s.Pod(ctx)
	if err != nil {
		return err
	}
	var syncErr error

	switch {
	case h.canApplyChanges(current, kvvm, kvvmi, pod, changes):
		cb := conditions.NewConditionBuilder2(vmcondition.TypeConfigurationApplied).
			Generation(current.GetGeneration())
		// No need to wait, apply changes to KVVM immediately.
		err = h.applyVMChangesToKVVM(ctx, s, changes)
		if err != nil {
			syncErr = errors.Join(syncErr, err)
			cb.Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonConfigurationNotApplied).
				Message(fmt.Sprintf("Failed to apply configuration changes: %s", err.Error()))
		} else {
			changed.Status.RestartAwaitingChanges = nil
			cb.Status(metav1.ConditionTrue).
				Reason2(vmcondition.ReasonConfigurationApplied)
		}
		mgr.Update(cb.Condition())
		mgr.Update(conditions.NewConditionBuilder2(vmcondition.TypeAwaitingRestartToApplyConfiguration).
			Generation(current.GetGeneration()).
			Reason2(vmcondition.ReasonRestartNoNeed).
			Status(metav1.ConditionFalse).
			Condition())
		// Changes are applied, consider current spec as last applied.
		lastAppliedSpec = &current.Spec
	case !changes.IsEmpty():
		// Delay changes propagation to KVVM until user restarts VM.
		cb := conditions.NewConditionBuilder2(vmcondition.TypeAwaitingRestartToApplyConfiguration).
			Generation(current.GetGeneration())

		var statusChanges []apiextensionsv1.JSON
		statusChanges, err = changes.ConvertPendingChanges()
		if err != nil {
			cb.Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonRestartAwaitingChangesNotExist).
				Message(fmt.Sprintf("Failed to generate RestartAwaitingChanges: %s", err.Error()))
			log.Error(fmt.Sprintf("Error should not occurs when preparing changesInfo, there is a possible bug in code: %v", syncErr))
			syncErr = errors.Join(syncErr, fmt.Errorf("convert pending changes for status: %w", syncErr))
		} else {
			cb.Status(metav1.ConditionTrue).
				Reason2(vmcondition.ReasonRestartAwaitingChangesExist)
		}
		mgr.Update(cb.Condition())
		mgr.Update(conditions.NewConditionBuilder2(vmcondition.TypeConfigurationApplied).
			Generation(current.GetGeneration()).
			Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonConfigurationNotApplied).
			Message("Waiting for restart from user.").
			Condition())
		changed.Status.RestartAwaitingChanges = statusChanges
	default:
		mgr.Update(conditions.NewConditionBuilder2(vmcondition.TypeConfigurationApplied).
			Generation(current.GetGeneration()).
			Status(metav1.ConditionTrue).
			Reason2(vmcondition.ReasonConfigurationApplied).
			Condition())
		mgr.Update(conditions.NewConditionBuilder2(vmcondition.TypeAwaitingRestartToApplyConfiguration).
			Generation(current.GetGeneration()).
			Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonRestartNoNeed).
			Condition())
		changed.Status.RestartAwaitingChanges = nil
	}
	changed.Status.Conditions = mgr.Generate()
	// Ensure power state according to the runPolicy.
	err = h.syncPowerState(ctx, s, kvvm, kvvmi, lastAppliedSpec)
	if err != nil {
		log.Error(fmt.Sprintf("Failed to sync powerstate for VirtualMachine %q: %v", current.GetName(), err))
		syncErr = errors.Join(syncErr, fmt.Errorf("failed to sync powerstate: %w", err))
	}

	return syncErr
}

// createKVVM constructs and creates new KubeVirt VirtualMachine based on d8 VirtualMachine spec.
func (h *SyncKvvmHandler) createKVVM(ctx context.Context, s state.VirtualMachineState) error {
	log := logger.FromContext(ctx)

	if s.VirtualMachine().IsEmpty() {
		return fmt.Errorf("VM is empty")
	}
	kvvm, err := h.makeKVVMFromVMSpec(ctx, s)
	current := s.VirtualMachine().Current()
	if err != nil {
		return fmt.Errorf("prepare to create KubeVirt VM '%s': %w", current.GetName(), err)
	}

	if err = h.client.Create(ctx, kvvm); err != nil {
		return fmt.Errorf("unable to create KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	log.Info("Created new KubeVirt VM", "name", kvvm.Name)
	log.Debug("Created new KubeVirt VM", "name", kvvm.Name, "kvvm", kvvm)

	return nil
}

// updateKVVM constructs and creates new KubeVirt VirtualMachine based on d8 VirtualMachine spec.
func (h *SyncKvvmHandler) updateKVVM(ctx context.Context, s state.VirtualMachineState) error {
	log := logger.FromContext(ctx)

	if s.VirtualMachine().IsEmpty() {
		return fmt.Errorf("VM is empty")
	}
	kvvm, err := h.makeKVVMFromVMSpec(ctx, s)
	current := s.VirtualMachine().Current()
	if err != nil {
		return fmt.Errorf("prepare to update KubeVirt VM '%s': %w", current.GetName(), err)
	}

	if err = h.client.Update(ctx, kvvm); err != nil {
		return fmt.Errorf("unable to create KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	log.Info("Update KubeVirt VM done", "name", kvvm.Name)
	log.Debug("Update KubeVirt VM done", "name", kvvm.Name, "kvvm", kvvm)

	return nil
}

// restartKVVM deletes KVVMI to restart VM.
func (h *SyncKvvmHandler) restartKVVM(ctx context.Context, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) error {
	err := powerstate.RestartVM(ctx, h.client, kvvm, kvvmi, false)
	if err != nil {
		return fmt.Errorf("unable to restart current KubeVirt VMI %q: %w", kvvmi.GetName(), err)
	}
	return nil
}

func (h *SyncKvvmHandler) makeKVVMFromVMSpec(ctx context.Context, s state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
	if s.VirtualMachine().IsEmpty() {
		return nil, nil
	}
	current := s.VirtualMachine().Current()
	kvvmName := common.NamespacedName(current)

	kvvmOpts := kvbuilder.KVVMOptions{
		EnableParavirtualization:  current.Spec.EnableParavirtualization,
		OsType:                    current.Spec.OsType,
		ForceBridgeNetworkBinding: os.Getenv("FORCE_BRIDGE_NETWORK_BINDING") == "1",
		DisableHypervSyNIC:        os.Getenv("DISABLE_HYPERV_SYNIC") == "1",
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
		return nil, fmt.Errorf("failed to relaod blockdevice state for vm %q: %w", current.GetName(), err)
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
		return nil, fmt.Errorf("the IP address is not found for VM %q", current.GetName())
	}

	// Create kubevirt VirtualMachine resource from d8 VirtualMachine spec.
	err = kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, current, bdState.VDByName, bdState.VIByName, bdState.CVIByName, h.dvcrSettings, class, ip.Status.Address)
	if err != nil {
		return nil, err
	}
	newKVVM := kvvmBuilder.GetResource()

	err = kvbuilder.SetLastAppliedSpec(newKVVM, current)
	if err != nil {
		return nil, fmt.Errorf("set last applied spec on KubeVirt VM '%s': %w", newKVVM.GetName(), err)
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

// detectSpecChanges compares KVVM generated from current VM spec with in cluster KVVM
// to calculate changes and action needed to apply these changes.
func (h *SyncKvvmHandler) detectSpecChanges(ctx context.Context, kvvm *virtv1.VirtualMachine, currentSpec, lastSpec *virtv2.VirtualMachineSpec) vmchange.SpecChanges {
	log := logger.FromContext(ctx)

	// Not applicable if KVVM is absent.
	if kvvm == nil || lastSpec == nil {
		return vmchange.SpecChanges{}
	}

	// Compare VM spec applied to the underlying KVVM
	// with the current VM spec (maybe edited by the user).
	specChanges := vmchange.CompareSpecs(lastSpec, currentSpec)

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
		if err := h.updateKVVM(ctx, s); err != nil {
			return fmt.Errorf("unable to update KVVM using new VM spec: %w", err)
		}
		// Ask kubevirt to re-create KVVMI to apply new spec from KVVM.
		if err := h.restartKVVM(ctx, kvvm, kvvmi); err != nil {
			return fmt.Errorf("unable restart KVVM instance in order to apply changes: %w", err)
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

		if err := h.updateKVVMLastAppliedSpec(ctx, current, kvvm); err != nil {
			return fmt.Errorf("unable to update last-applied-spec on KVVM: %w", err)
		}
	}
	return nil
}

// updateKVVMLastAppliedSpec updates last-applied-spec annotation on KubeVirt VirtualMachine.
func (h *SyncKvvmHandler) updateKVVMLastAppliedSpec(ctx context.Context, vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) error {
	if vm == nil || kvvm == nil {
		return nil
	}

	err := kvbuilder.SetLastAppliedSpec(kvvm, vm)
	if err != nil {
		return fmt.Errorf("set last applied spec on KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	if err := h.client.Update(ctx, kvvm); err != nil {
		return fmt.Errorf("unable to update KubeVirt VM '%s': %w", kvvm.GetName(), err)
	}

	log := logger.FromContext(ctx)
	log.Info("Update last applied spec on KubeVirt VM done", "name", kvvm.GetName())

	return nil
}

// syncPowerState enforces runPolicy on the underlying KVVM.
func (h *SyncKvvmHandler) syncPowerState(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, effectiveSpec *virtv2.VirtualMachineSpec) error {
	log := logger.FromContext(ctx)

	if kvvm == nil {
		return nil
	}

	vmRunPolicy := effectiveSpec.RunPolicy
	var shutdownInfo powerstate.ShutdownInfo
	s.Shared(func(s *state.Shared) {
		shutdownInfo = s.ShutdownInfo
	})

	var err error
	switch vmRunPolicy {
	case virtv2.AlwaysOffPolicy:
		if kvvmi != nil {
			// Ensure KVVMI is absent.
			err = h.client.Delete(ctx, kvvmi)
			if err != nil && !k8serrors.IsNotFound(err) {
				return fmt.Errorf("force AlwaysOff: delete KVVMI: %w", err)
			}
		}
		err = h.ensureRunStrategy(ctx, kvvm, virtv1.RunStrategyHalted)
	case virtv2.AlwaysOnPolicy:
		// Power state change reason is not significant for AlwaysOn:
		// kubevirt restarts VM via re-creation of KVVMI.
		err = h.ensureRunStrategy(ctx, kvvm, virtv1.RunStrategyAlways)
	case virtv2.AlwaysOnUnlessStoppedManually:
		strategy, _ := kvvm.RunStrategy()
		if strategy == virtv1.RunStrategyAlways && kvvmi == nil {
			if err = powerstate.StartVM(ctx, h.client, kvvm); err != nil {
				return fmt.Errorf("failed to start VM: %w", err)
			}
		}
		if kvvmi != nil && kvvmi.DeletionTimestamp == nil {
			if kvvmi.Status.Phase == virtv1.Succeeded {
				if shutdownInfo.PodCompleted {
					// Request to start new KVVMI if guest was restarted.
					// Cleanup KVVMI is enough if VM was stopped from inside.
					switch shutdownInfo.Reason {
					case powerstate.GuestResetReason:
						log.Info("Restart for guest initiated reset")
						err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
						if err != nil {
							return fmt.Errorf("restart VM on guest-reset: %w", err)
						}
					default:
						log.Info("Cleanup Succeeded KVVMI")
						err = h.client.Delete(ctx, kvvmi)
						if err != nil && !k8serrors.IsNotFound(err) {
							return fmt.Errorf("delete Succeeded KVVMI: %w", err)
						}
					}
				}
			}
			if kvvmi.Status.Phase == virtv1.Failed {
				log.Info("Restart on Failed KVVMI", "obj", kvvmi.GetName())
				err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
				if err != nil {
					return fmt.Errorf("restart VM on failed: %w", err)
				}
			}
		}

		err = h.ensureRunStrategy(ctx, kvvm, virtv1.RunStrategyManual)
	case virtv2.ManualPolicy:
		// Manual policy requires to handle only guest-reset event.
		// All types of shutdown are a final state.
		if kvvmi != nil && kvvmi.DeletionTimestamp == nil {
			if kvvmi.Status.Phase == virtv1.Succeeded && shutdownInfo.PodCompleted {
				// Request to start new KVVMI (with updated settings).
				switch shutdownInfo.Reason {
				case powerstate.GuestResetReason:
					err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
					if err != nil {
						return fmt.Errorf("restart VM on guest-reset: %w", err)
					}
				default:
					// Cleanup old version of KVVMI.
					log.Info("Cleanup Succeeded KVVMI")
					err = h.client.Delete(ctx, kvvmi)
					if err != nil && !k8serrors.IsNotFound(err) {
						return fmt.Errorf("delete Succeeded KVVMI: %w", err)
					}
				}
			}
		}

		err = h.ensureRunStrategy(ctx, kvvm, virtv1.RunStrategyManual)
	}

	if err != nil {
		return fmt.Errorf("enforce runPolicy %s: %w", vmRunPolicy, err)
	}

	return nil
}

func (h *SyncKvvmHandler) ensureRunStrategy(ctx context.Context, kvvm *virtv1.VirtualMachine, desiredRunStrategy virtv1.VirtualMachineRunStrategy) error {
	if kvvm == nil {
		return nil
	}
	kvvmRunStrategy := kvvmutil.GetRunStrategy(kvvm)

	if kvvmRunStrategy == desiredRunStrategy {
		return nil
	}
	patch := kvvmutil.PatchRunStrategy(desiredRunStrategy)
	err := h.client.Patch(ctx, kvvm, patch)
	if err != nil {
		return fmt.Errorf("patch KVVM with runStrategy %s: %w", desiredRunStrategy, err)
	}

	return nil
}
