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
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	vmutil "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameSyncKvvmHandler = "SyncKvvmHandler"

type syncVolumesService interface {
	SyncVolumes(ctx context.Context, s state.VirtualMachineState, restartRequired bool) (reconcile.Result, error)
}

func NewSyncKvvmHandler(
	dvcrSettings *dvcr.Settings,
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	featureGate featuregate.FeatureGate,
	syncVolumesService syncVolumesService,
) *SyncKvvmHandler {
	return &SyncKvvmHandler{
		dvcrSettings:       dvcrSettings,
		client:             client,
		recorder:           recorder,
		featureGate:        featureGate,
		syncVolumesService: syncVolumesService,
	}
}

type SyncKvvmHandler struct {
	client             client.Client
	recorder           eventrecord.EventRecorderLogger
	dvcrSettings       *dvcr.Settings
	featureGate        featuregate.FeatureGate
	syncVolumesService syncVolumesService
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
		Reason(vmcondition.ReasonNoRestartRequired)

	defer func() {
		switch changed.Status.Phase {
		case v1alpha2.MachinePending, v1alpha2.MachineStarting, v1alpha2.MachineStopped:
			conditions.RemoveCondition(vmcondition.TypeConfigurationApplied, &changed.Status.Conditions)
			conditions.RemoveCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, &changed.Status.Conditions)

		default:
			if cbConfApplied.Condition().Status == metav1.ConditionFalse {
				conditions.SetCondition(cbConfApplied, &changed.Status.Conditions)
			} else {
				conditions.RemoveCondition(vmcondition.TypeConfigurationApplied, &changed.Status.Conditions)
			}

			if cbAwaitingRestart.Condition().Status == metav1.ConditionTrue {
				conditions.SetCondition(cbAwaitingRestart, &changed.Status.Conditions)
			} else {
				conditions.RemoveCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, &changed.Status.Conditions)
			}
		}
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
	var (
		lastAppliedSpec *v1alpha2.VirtualMachineSpec
		changes         vmchange.SpecChanges
		allChanges      vmchange.SpecChanges
		classChanged    bool
	)
	if kvvm != nil {
		lastAppliedSpec = h.loadLastAppliedSpec(current, kvvm)
		lastClassAppliedSpec := h.loadClassLastAppliedSpec(class, kvvm)
		changes = h.detectSpecChanges(ctx, kvvm, &current.Spec, lastAppliedSpec)
		if !changes.IsEmpty() {
			allChanges.Add(changes.GetAll()...)
		}
		if class != nil {
			classChanges := h.detectClassSpecChanges(ctx, &class.Spec, lastClassAppliedSpec)
			if !classChanges.IsEmpty() {
				allChanges.Add(classChanges.GetAll()...)
				classChanged = classChanges.IsDisruptive()
			}
		}
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
	synced, kvvmSyncErr := h.syncKVVM(ctx, s, allChanges)
	if kvvmSyncErr != nil {
		errs = errors.Join(errs, fmt.Errorf("failed to sync the internal virtual machine: %w", kvvmSyncErr))
	}

	if synced {
		// 3.1. Changes are applied, consider current spec as last applied.
		changed.Status.RestartAwaitingChanges = nil
	}

	// 4. Set ConfigurationApplied condition.
	switch {
	case kvvmSyncErr != nil:
		h.recorder.Event(current, corev1.EventTypeWarning, v1alpha2.ReasonErrVmNotSynced, kvvmSyncErr.Error())
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message(service.CapitalizeFirstLetter(kvvmSyncErr.Error()) + ".")
	case len(changed.Status.RestartAwaitingChanges) > 0:
		h.recorder.Event(current, corev1.EventTypeNormal, v1alpha2.ReasonErrRestartAwaitingChanges, "The virtual machine configuration successfully synced")
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message("Waiting for the user to restart in order to apply the configuration changes.")
		cbAwaitingRestart.
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonChangesPendingRestart).
			Message("Waiting for the user to restart in order to apply the configuration changes.")
	case classChanged:
		h.recorder.Event(current, corev1.EventTypeNormal, v1alpha2.ReasonErrRestartAwaitingChanges, "Restart required to propagate changes from the vmclass spec")
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message("VirtualMachineClass.spec has been modified. Waiting for the user to restart in order to apply the configuration changes.")
		cbAwaitingRestart.
			Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonChangesPendingRestart).
			Message("VirtualMachineClass.spec has been modified. Waiting for the user to restart in order to apply the configuration changes.")
	case synced:
		h.recorder.Event(current, corev1.EventTypeNormal, v1alpha2.ReasonErrVmSynced, "The virtual machine configuration successfully synced")
		cbConfApplied.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonConfigurationApplied)
	default:
		log.Error("Unexpected case during kvvm sync, please report a bug")
	}

	// 5. Set RestartRequired from KVVM condition.
	if cbAwaitingRestart.Condition().Status == metav1.ConditionFalse && kvvm != nil {
		// The check for StateChangeRequests is added to ignore the RestartRequired condition when it is set while
		// the virtual machine is in the process of rebooting.
		cond, _ := conditions.GetKVVMCondition(virtv1.VirtualMachineRestartRequired, kvvm.Status.Conditions)
		if cond.Status == corev1.ConditionTrue && len(kvvm.Status.StateChangeRequests) == 0 {
			msg := "Please restart the virtual machine to synchronize its configuration."
			log.Error(msg)
			cbAwaitingRestart.
				Status(metav1.ConditionTrue).
				Reason(vmcondition.ReasonUnexpectedState).
				Message(msg)
		}
	}

	// 6. Sync migrating volumes if needed.
	result, migrateVolumesErr := h.syncVolumesService.SyncVolumes(ctx, s, cbAwaitingRestart.Condition().Status == metav1.ConditionTrue)
	if migrateVolumesErr != nil {
		errs = errors.Join(errs, fmt.Errorf("failed to sync migrating volumes: %w", migrateVolumesErr))
	}
	return result, errs
}

func (h *SyncKvvmHandler) Name() string {
	return nameSyncKvvmHandler
}

func (h *SyncKvvmHandler) isWaiting(vm *v1alpha2.VirtualMachine) bool {
	return !virtualMachineDependenciesAreReady(vm)
}

func (h *SyncKvvmHandler) syncKVVM(ctx context.Context, s state.VirtualMachineState, allChanges vmchange.SpecChanges) (bool, error) {
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
	// This workaround is required due to a bug in the KVVM workflow.
	// When a KVVM is created with conflicting placement rules and cannot be scheduled,
	// it remains unschedulable even if these rules are changed or removed.
	case h.isVMUnschedulable(s.VirtualMachine().Current(), kvvm) && h.isPlacementPolicyChanged(allChanges):
		err := h.updateKVVM(ctx, s)
		if err != nil {
			return false, fmt.Errorf("failed to update internal virtual machine: %w", err)
		}
		err = object.DeleteObject(ctx, h.client, pod)
		if err != nil {
			return false, fmt.Errorf("failed to delete the internal virtual machine instance's pod: %w", err)
		}
		return true, nil
	case h.isVMStopped(s.VirtualMachine().Current(), kvvm, pod):
		// KVVM should be updated when VM become stopped.
		// It is safe to update KVVM at this point in general and also all related resources
		// can be changed during the restoration process: e.g. VirtualDisks, VMIPs, etc.
		// For example, the PVC of the VirtualDisk will be changed,
		//  and the volume with this PVC must be updated in the KVVM specification.
		err := h.updateKVVM(ctx, s)
		if err != nil {
			return false, fmt.Errorf("update internal virtual machine in 'Stopped' state: %w", err)
		}
		return true, nil
	case h.hasNoneDisruptiveChanges(s.VirtualMachine().Current(), kvvm, kvvmi, allChanges):
		// No need to wait, apply changes to KVVM immediately.
		err = h.applyVMChangesToKVVM(ctx, s, allChanges)
		if err != nil {
			return false, fmt.Errorf("apply changes to the internal virtual machine: %w", err)
		}

		return true, nil
	case allChanges.IsEmpty():
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
	kvvm, err := MakeKVVMFromVMSpec(ctx, s)
	if err != nil {
		return fmt.Errorf("failed to make the internal virtual machine: %w", err)
	}

	err = h.client.Create(ctx, kvvm)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			log.Warn("The KubeVirt VM already exists", "name", kvvm.Name)
			return nil
		}

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

	newKVVM, err := MakeKVVMFromVMSpec(ctx, s)
	if err != nil {
		return fmt.Errorf("update internal virtual machine: make kvvm from the virtual machine spec: %w", err)
	}

	currentKVVM, err := s.KVVM(ctx)
	if err != nil {
		return fmt.Errorf("get current kvvm: %w", err)
	}

	// Check for changes to skip unneeded updated.
	isChanged := IsKVVMChanged(currentKVVM, newKVVM)

	if isChanged {
		// Update can't handle proper reset of memory fields, so patch-after-update:
		// (1) make memory copy, (2) reset memory in newKVVM and (3) patch memory field after update.
		domainMemory := saveKVVMDomainMemoryForPatching(currentKVVM, newKVVM)
		if domainMemory != nil {
			newKVVM.Spec.Template.Spec.Domain.Memory = currentKVVM.Spec.Template.Spec.Domain.Memory
		}

		if err = h.client.Update(ctx, newKVVM); err != nil {
			return fmt.Errorf("update internal virtual machine: %w", err)
		}

		log.Info("Update internal virtual machine done", "name", newKVVM.Name)
		log.Debug("Update internal virtual machine done", "name", newKVVM.Name, "kvvm", newKVVM)

		if domainMemory != nil {
			jsonPatch := patch.JSONPatch{}
			// Removing memory.maxGuest is not enough, replace memory.guest is needed to pass the vm-validator webhook.
			jsonPatch.Append(
				patch.WithRemove("/spec/template/spec/domain/memory/maxGuest"),
				patch.WithReplace("/spec/template/spec/domain/memory/guest", domainMemory.Guest.String()),
			)
			patchBytes, err := jsonPatch.Bytes()
			if err != nil {
				return fmt.Errorf("prepare json patch for internal virtual machine: %w", err)
			}
			if err = h.client.Patch(ctx, newKVVM, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
				return fmt.Errorf("patch internal virtual machine before update: %w", err)
			}
		}
	} else {
		log.Debug("Update internal virtual machine is not needed", "name", newKVVM.Name, "kvvm", newKVVM)
	}

	return nil
}

// saveKVVMDomainMemoryForPatching returns copy of domain memory if maxGuest becomes 0.
//
// Note: maxGuest=0 is an invalid value for the vm-validator webhook,
// kvbuilder sets maxGuest to 0 to indicate that KVVM needs to be patched
// to clear maxGuest value: it is not possible to clear the value with the Update
// once it was set previously.
func saveKVVMDomainMemoryForPatching(prevKVVM, newKVVM *virtv1.VirtualMachine) *virtv1.Memory {
	prevMemory := prevKVVM.Spec.Template.Spec.Domain.Memory
	newMemory := newKVVM.Spec.Template.Spec.Domain.Memory
	if newMemory != nil && newMemory.MaxGuest != nil && newMemory.MaxGuest.IsZero() &&
		prevMemory != nil && prevMemory.MaxGuest != nil && !prevMemory.MaxGuest.IsZero() {
		return newMemory.DeepCopy()
	}
	return nil
}

func MakeKVVMFromVMSpec(ctx context.Context, s state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
	if s.VirtualMachine().IsEmpty() {
		return nil, nil
	}
	current := s.VirtualMachine().Current()
	kvvmName := object.NamespacedName(current)

	kvvmOpts := kvbuilder.DefaultOptions(current)

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
		return nil, fmt.Errorf("failed to reload blockdevice state for the virtual machine: %w", err)
	}
	class, err := s.Class(ctx)
	if err != nil {
		return nil, err
	}
	ip, err := s.IPAddress(ctx)
	if err != nil {
		return nil, err
	}

	ipAddress := ""
	if ip != nil {
		if ip.Status.Address == "" {
			return nil, fmt.Errorf("the IP address is not found for the virtual machine")
		} else {
			ipAddress = ip.Status.Address
		}
	}

	vmmacs, err := s.VirtualMachineMACAddresses(ctx)
	if err != nil {
		return nil, err
	}

	networkSpec := network.CreateNetworkSpec(current, vmmacs)

	// Create kubevirt VirtualMachine resource from d8 VirtualMachine spec.
	err = kvbuilder.ApplyVirtualMachineSpec(kvvmBuilder, current, bdState.VDByName, bdState.VIByName, bdState.CVIByName, class, ipAddress, networkSpec)
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

// IsKVVMChanged returns whether kvvm spec or special annotations are changed.
func IsKVVMChanged(prevKVVM, newKVVM *virtv1.VirtualMachine) bool {
	if prevKVVM.Annotations[annotations.AnnVMLastAppliedSpec] != newKVVM.Annotations[annotations.AnnVMLastAppliedSpec] {
		return true
	}

	if prevKVVM.Annotations[annotations.AnnVMClassLastAppliedSpec] != newKVVM.Annotations[annotations.AnnVMClassLastAppliedSpec] {
		return true
	}

	return !reflect.DeepEqual(prevKVVM.Spec, newKVVM.Spec)
}

func (h *SyncKvvmHandler) loadLastAppliedSpec(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) *v1alpha2.VirtualMachineSpec {
	if kvvm == nil || vm == nil {
		return nil
	}

	lastSpec, err := kvbuilder.LoadLastAppliedSpec(kvvm)
	// TODO Add smarter handler for empty/invalid annotation.
	if lastSpec == nil && err == nil {
		h.recorder.Event(vm, corev1.EventTypeWarning, v1alpha2.ReasonVMLastAppliedSpecIsInvalid, "Could not find last applied spec. Possible old VM or partial backup restore. Restart or recreate VM to adopt it.")
		lastSpec = &v1alpha2.VirtualMachineSpec{}
	}
	if err != nil {
		msg := fmt.Sprintf("Could not restore last applied spec: %v. Possible old VM or partial backup restore. Restart or recreate VM to adopt it.", err)
		h.recorder.Event(vm, corev1.EventTypeWarning, v1alpha2.ReasonVMLastAppliedSpecIsInvalid, msg)
		// In Automatic mode changes are applied immediately, so last-applied-spec annotation will be restored.
		if vmutil.ApprovalMode(vm) == v1alpha2.Automatic {
			lastSpec = &v1alpha2.VirtualMachineSpec{}
		}
		if vmutil.ApprovalMode(vm) == v1alpha2.Manual {
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
			lastSpec = &v1alpha2.VirtualMachineSpec{}
		}
	}

	return lastSpec
}

func (h *SyncKvvmHandler) loadClassLastAppliedSpec(class *v1alpha2.VirtualMachineClass, kvvm *virtv1.VirtualMachine) *v1alpha2.VirtualMachineClassSpec {
	if kvvm == nil || class == nil {
		return nil
	}

	lastSpec, err := kvbuilder.LoadLastAppliedClassSpec(kvvm)
	// TODO Add smarter handler for empty/invalid annotation.
	if lastSpec == nil && err == nil {
		h.recorder.Event(class, corev1.EventTypeWarning, v1alpha2.ReasonVMClassLastAppliedSpecInvalid, "Could not find last applied spec. Possible old VMClass or partial backup restore. Restart or recreate VM to adopt it.")
		lastSpec = &v1alpha2.VirtualMachineClassSpec{}
	}
	if err != nil {
		msg := fmt.Sprintf("Could not restore last applied spec: %v. Possible old VMClass or partial backup restore. Restart or recreate VM to adopt it.", err)
		h.recorder.Event(class, corev1.EventTypeWarning, v1alpha2.ReasonVMClassLastAppliedSpecInvalid, msg)
		lastSpec = &v1alpha2.VirtualMachineClassSpec{}
	}

	return lastSpec
}

// detectSpecChanges compares KVVM generated from current VM spec with in cluster KVVM
// to calculate changes and action needed to apply these changes.
func (h *SyncKvvmHandler) detectSpecChanges(
	ctx context.Context,
	kvvm *virtv1.VirtualMachine,
	currentSpec, lastSpec *v1alpha2.VirtualMachineSpec,
) vmchange.SpecChanges {
	log := logger.FromContext(ctx)

	// Not applicable if KVVM is absent.
	if kvvm == nil || lastSpec == nil {
		return vmchange.SpecChanges{}
	}

	// Compare VM spec applied to the underlying KVVM
	// with the current VM spec (maybe edited by the user).
	specChanges := vmchange.NewVMSpecComparator(h.featureGate).Compare(lastSpec, currentSpec)

	log.Info(fmt.Sprintf("detected VM changes: empty %v, disruptive %v, actionType %v", specChanges.IsEmpty(), specChanges.IsDisruptive(), specChanges.ActionType()))
	log.Info(fmt.Sprintf("detected VM changes JSON: %s", specChanges.ToJSON()))

	return specChanges
}

func (h *SyncKvvmHandler) detectClassSpecChanges(ctx context.Context, currentClassSpec, lastClassSpec *v1alpha2.VirtualMachineClassSpec) vmchange.SpecChanges {
	log := logger.FromContext(ctx)

	specChanges := vmchange.CompareClassSpecs(currentClassSpec, lastClassSpec)

	log.Info(fmt.Sprintf("detected VMClass changes: empty %v, disruptive %v, actionType %v", specChanges.IsEmpty(), specChanges.IsDisruptive(), specChanges.ActionType()))
	log.Info(fmt.Sprintf("detected VMClass changes JSON: %s", specChanges.ToJSON()))

	return specChanges
}

// IsVmStopped return true if the instance of the KVVM is not created or Pod is in the Complete state.
func (h *SyncKvvmHandler) isVMStopped(
	vm *v1alpha2.VirtualMachine,
	kvvm *virtv1.VirtualMachine,
	pod *corev1.Pod,
) bool {
	if vm == nil {
		return false
	}

	podStopped := true
	if pod != nil {
		phase := pod.Status.Phase
		podStopped = phase != corev1.PodPending && phase != corev1.PodRunning
	}

	return isVMStopped(kvvm) && (!isKVVMICreated(kvvm) || podStopped)
}

// canApplyChanges returns true if changes can be applied right now.
//
// Wait if changes are disruptive, and approval mode is manual, and VM is still running.
func (h *SyncKvvmHandler) hasNoneDisruptiveChanges(
	vm *v1alpha2.VirtualMachine,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	changes vmchange.SpecChanges,
) bool {
	if vm == nil || changes.IsEmpty() {
		return false
	}
	if !changes.IsDisruptive() || kvvmi == nil {
		return true
	}

	if isVMPending(kvvm) {
		return true
	}

	return false
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
		// Update KVVM spec according the current VM spec.
		if err = h.updateKVVM(ctx, s); err != nil {
			return fmt.Errorf("update virtual machine instance with new spec: %w", err)
		}

	case vmchange.ActionApplyImmediate:
		message := "Apply changes without restart"
		if changes.IsDisruptive() {
			message = "Apply disruptive changes without restart"
		}
		h.recorder.Event(current, corev1.EventTypeNormal, v1alpha2.ReasonVMChangesApplied, message)
		log.Debug(message, "vm.name", current.GetName(), "changes", changes)

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
	vm *v1alpha2.VirtualMachine,
	kvvm *virtv1.VirtualMachine,
	class *v1alpha2.VirtualMachineClass,
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

func (h *SyncKvvmHandler) isVMUnschedulable(
	vm *v1alpha2.VirtualMachine,
	kvvm *virtv1.VirtualMachine,
) bool {
	if vm.Status.Phase == v1alpha2.MachinePending && kvvm.Status.PrintableStatus == virtv1.VirtualMachineStatusUnschedulable {
		return true
	}

	return false
}

// isPlacementPolicyChanged returns true if any of the Affinity, NodePlacement, or Toleration rules have changed.
func (h *SyncKvvmHandler) isPlacementPolicyChanged(allChanges vmchange.SpecChanges) bool {
	for _, c := range allChanges.GetAll() {
		switch c.Path {
		case "affinity", "nodeSelector", "tolerations":
			if !equality.Semantic.DeepEqual(c.CurrentValue, c.DesiredValue) {
				return true
			}
		}
	}

	return false
}
