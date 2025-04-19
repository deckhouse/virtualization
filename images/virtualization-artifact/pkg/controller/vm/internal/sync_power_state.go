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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const nameSyncPowerStateHandler = "SyncPowerStateHandler"

type VMAction int

const (
	Nothing VMAction = iota
	Start
	Stop
	Restart
)

func NewSyncPowerStateHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *SyncPowerStateHandler {
	return &SyncPowerStateHandler{
		client:   client,
		recorder: recorder,
	}
}

type SyncPowerStateHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func (h *SyncPowerStateHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log, ctx := logger.GetHandlerContext(ctx, nameSyncPowerStateHandler)

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachine().Changed()
	if isDeletion(changed) {
		return reconcile.Result{}, nil
	}

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("find the internal virtual machine: %w", err)
	}

	err = h.syncPowerState(ctx, s, kvvm, changed.Spec.RunPolicy)
	if err != nil {
		err = fmt.Errorf("failed to sync powerstate: %w", err)
		log.Error(err.Error())
	}

	return reconcile.Result{}, err
}

// syncPowerState enforces runPolicy on the underlying KVVM.
func (h *SyncPowerStateHandler) syncPowerState(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	runPolicy virtv2.RunPolicy,
) error {
	if kvvm == nil {
		return nil
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return fmt.Errorf("find the virtual machine instance: %w", err)
	}

	if runPolicy == virtv2.AlwaysOnUnlessStoppedManually {
		if kvvmi != nil {
			err = h.ensureRunStrategy(ctx, kvvm, virtv1.RunStrategyManual)
		} else if kvvm.Spec.RunStrategy != nil && *kvvm.Spec.RunStrategy == virtv1.RunStrategyAlways {
			h.recordStartEventf(ctx, s.VirtualMachine().Current(),
				"Start on create initiated by controller for %v policy",
				runPolicy,
			)
		}
	} else {
		err = h.ensureRunStrategy(ctx, kvvm, virtv1.RunStrategyManual)
	}

	if err != nil {
		return fmt.Errorf("enforce runPolicy %s: %w", runPolicy, err)
	}

	var shutdownInfo powerstate.ShutdownInfo
	s.Shared(func(s *state.Shared) {
		shutdownInfo = s.ShutdownInfo
	})

	isConfigurationApplied := checkVirtualMachineConfiguration(s.VirtualMachine().Changed())
	var vmAction VMAction
	switch runPolicy {
	case virtv2.AlwaysOffPolicy:
		vmAction = h.handleAlwaysOffPolicy(ctx, s, kvvmi)
	case virtv2.AlwaysOnPolicy:
		vmAction, err = h.handleAlwaysOnPolicy(ctx, s, kvvm, kvvmi, isConfigurationApplied, shutdownInfo)
		if err != nil {
			return err
		}
	case virtv2.AlwaysOnUnlessStoppedManually:
		vmAction, err = h.handleAlwaysOnUnlessStoppedManuallyPolicy(ctx, s, kvvm, kvvmi, isConfigurationApplied, shutdownInfo)
		if err != nil {
			return err
		}
	case virtv2.ManualPolicy:
		vmAction = h.handleManualPolicy(ctx, s, kvvm, kvvmi, isConfigurationApplied, shutdownInfo)
	}

	switch vmAction {
	case Nothing:
		return nil
	case Start:
		return h.start(ctx, s, kvvm, isConfigurationApplied)
	case Stop:
		return h.deleteKVVMI(ctx, kvvmi)
	case Restart:
		return h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
	}

	return nil
}

func (h *SyncPowerStateHandler) handleAlwaysOffPolicy(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvmi *virtv1.VirtualMachineInstance,
) VMAction {
	if kvvmi != nil {
		h.recordStopEventf(ctx, s.VirtualMachine().Current(),
			"Stop initiated by controller to ensure AlwaysOff policy",
		)
		return Stop
	}

	return Nothing
}

func (h *SyncPowerStateHandler) handleManualPolicy(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	isConfigurationApplied bool,
	shutdownInfo powerstate.ShutdownInfo,
) VMAction {
	if kvvmi == nil || kvvmi.DeletionTimestamp != nil {
		if h.checkNeedStartVM(ctx, s, kvvm, isConfigurationApplied, virtv2.ManualPolicy) {
			return Start
		}
		return Nothing
	}

	if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" && kvvmi.Status.Phase == virtv1.Running {
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated "+
			"by VirtualMachineOparation for Manual runPolicy")
		return Restart
	} else if kvvmi.Status.Phase == virtv1.Succeeded && shutdownInfo.PodCompleted {
		if shutdownInfo.Reason == powerstate.GuestResetReason {
			h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by inside "+
				"the guest VirtualMachine for Manual runPolicy")
			return Restart
		} else {
			h.recordStopEventf(ctx, s.VirtualMachine().Current(), "Stop initiated from inside "+
				"the guest VirtualMachine")
			return Stop
		}
	}

	return Nothing
}

func (h *SyncPowerStateHandler) isVMRestarting(kvvm *virtv1.VirtualMachine) bool {
	if kvvm != nil &&
		len(kvvm.Status.StateChangeRequests) == 2 &&
		kvvm.Status.StateChangeRequests[0].Action == virtv1.StopRequest &&
		kvvm.Status.StateChangeRequests[1].Action == virtv1.StartRequest {
		return true
	}

	return false
}

func (h *SyncPowerStateHandler) handleAlwaysOnPolicy(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	isConfigurationApplied bool,
	shutdownInfo powerstate.ShutdownInfo,
) (VMAction, error) {
	if kvvmi == nil {
		if h.isVMRestarting(kvvm) {
			return Nothing, nil
		}

		if isConfigurationApplied {
			h.recordStartEventf(ctx, s.VirtualMachine().Current(), "Start initiated "+
				"by controller for AlwaysOn policy")
			return Start, nil
		}

		err := kvvmutil.AddStartAnnotation(ctx, h.client, kvvm)
		if err != nil {
			return Nothing, fmt.Errorf("add annotation to KVVM: %w", err)
		}

		return Nothing, nil
	}

	if kvvmi.DeletionTimestamp != nil {
		if h.checkNeedStartVM(ctx, s, kvvm, isConfigurationApplied, virtv2.AlwaysOnPolicy) {
			return Start, nil
		}
		return Nothing, nil
	}

	if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" && kvvmi.Status.Phase == virtv1.Running {
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated "+
			"by VirtualMachineOparation for AlwaysOn runPolicy")
		return Restart, nil
	}

	if kvvmi.Status.Phase == virtv1.Succeeded || kvvmi.Status.Phase == virtv1.Failed {
		if shutdownInfo.PodCompleted {
			if shutdownInfo.Reason == powerstate.GuestResetReason {
				h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by inside "+
					"the guest VirtualMachine for AlwaysOn runPolicy")
				return Restart, nil
			}
		}
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by controller "+
			"after stopping from inside the guest VirtualMachine for AlwaysOn runPolicy")
		return Restart, nil
	}

	return Nothing, nil
}

func (h *SyncPowerStateHandler) handleAlwaysOnUnlessStoppedManuallyPolicy(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	isConfigurationApplied bool,
	shutdownInfo powerstate.ShutdownInfo,
) (VMAction, error) {
	if kvvmi == nil || kvvmi.DeletionTimestamp != nil {
		if h.checkNeedStartVM(ctx, s, kvvm, isConfigurationApplied, virtv2.AlwaysOnUnlessStoppedManually) {
			return Start, nil
		}

		if kvvm != nil {
			lastAppliedSpec, err := kvbuilder.LoadLastAppliedSpec(kvvm)
			if err != nil {
				return Nothing, fmt.Errorf("load last applied spec: %w", err)
			}

			if lastAppliedSpec != nil && lastAppliedSpec.RunPolicy == virtv2.AlwaysOffPolicy {
				err = kvvmutil.AddStartAnnotation(ctx, h.client, kvvm)
				if err != nil {
					return Nothing, fmt.Errorf("add annotation to KVVM: %w", err)
				}
			}
		}

		return Nothing, nil
	}

	if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" && kvvmi.Status.Phase == virtv1.Running {
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by "+
			"VirtualMachineOparation for AlwaysOnUnlessStoppedManually runPolicy")
		return Restart, nil
	}

	vmPod, err := s.Pod(ctx)
	if err != nil {
		return Nothing, fmt.Errorf("get virtual machine pod: %w", err)
	}

	switch kvvmi.Status.Phase {
	case virtv1.Succeeded:
		if shutdownInfo.PodCompleted {
			if shutdownInfo.Reason == powerstate.GuestResetReason {
				h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by inside "+
					"the guest VirtualMachine for AlwaysOnUnlessStoppedManually runPolicy")
				return Restart, nil
			} else {
				if vmPod == nil || !vmPod.GetObjectMeta().GetDeletionTimestamp().IsZero() {
					h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by "+
						"controller after the deletion of pod VirtualMachine for AlwaysOnUnlessStoppedManually runPolicy")
					return Restart, nil
				}
				h.recordStopEventf(ctx, s.VirtualMachine().Current(), "Stop initiated from inside "+
					"the guest VirtualMachine")
				return Stop, nil
			}
		}

		if vmPod == nil {
			return Nothing, fmt.Errorf("failed to find VM pod")
		}
	case virtv1.Failed:
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by controller after "+
			"observing failed guest VirtualMachine for AlwaysOnUnlessStoppedManually runPolicy")
		return Restart, nil
	default:
		return Nothing, nil
	}

	return Nothing, nil
}

func (h *SyncPowerStateHandler) checkNeedStartVM(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	isConfigurationApplied bool,
	runPolicy virtv2.RunPolicy,
) bool {
	if isConfigurationApplied &&
		(kvvm.Annotations[annotations.AnnVmStartRequested] == "true" || kvvm.Annotations[annotations.AnnVmRestartRequested] == "true") {
		h.recordStartEventf(ctx, s.VirtualMachine().Current(), "Start initiated by controller for %v policy", runPolicy)
		return true
	}

	return false
}

func (h *SyncPowerStateHandler) start(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	isConfigurationApplied bool,
) error {
	if !isConfigurationApplied {
		h.recordStopEventf(ctx, s.VirtualMachine().Current(),
			"The virtual machine startup was interrupted because the provided configuration could not be applied.",
		)
		return h.interruptRunningVM(ctx, kvvm, nil)
	}

	if err := powerstate.StartVM(ctx, h.client, kvvm); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	if err := kvvmutil.RemoveStartAnnotation(ctx, h.client, kvvm); err != nil {
		return err
	}

	return kvvmutil.RemoveRestartAnnotation(ctx, h.client, kvvm)
}

func (h *SyncPowerStateHandler) restart(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	isConfigurationApplied bool,
) error {
	if !isConfigurationApplied {
		h.recordStopEventf(ctx, s.VirtualMachine().Current(),
			"The virtual machine startup was interrupted because the provided configuration could not be applied.",
		)
		return h.interruptRunningVM(ctx, kvvm, kvvmi)
	}

	if err := powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi); err != nil {
		return fmt.Errorf("restart VM: %w", err)
	}

	if err := kvvmutil.RemoveStartAnnotation(ctx, h.client, kvvm); err != nil {
		return err
	}

	return kvvmutil.RemoveRestartAnnotation(ctx, h.client, kvvm)
}

func (h *SyncPowerStateHandler) deleteKVVMI(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) error {
	err := h.client.Delete(ctx, kvvmi)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete KVVMI: %w", err)
	}
	return nil
}

func (h *SyncPowerStateHandler) interruptRunningVM(
	ctx context.Context,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
) error {
	if kvvmi != nil {
		err := h.client.Delete(ctx, kvvmi)
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("stop VM: delete KVVMI: %w", err)
		}
	}

	err := kvvmutil.AddStartAnnotation(ctx, h.client, kvvm)
	if err != nil {
		return fmt.Errorf("add annotation to KVVM: %w", err)
	}
	return nil
}

func (h *SyncPowerStateHandler) ensureRunStrategy(
	ctx context.Context,
	kvvm *virtv1.VirtualMachine,
	desiredRunStrategy virtv1.VirtualMachineRunStrategy,
) error {
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

func (h *SyncPowerStateHandler) Name() string {
	return nameSyncPowerStateHandler
}

func (h *SyncPowerStateHandler) recordStartEventf(ctx context.Context, obj client.Object, messageFmt string, args ...any) {
	h.recorder.WithLogging(logger.FromContext(ctx)).Eventf(
		obj,
		corev1.EventTypeNormal,
		virtv2.ReasonVMStarted,
		messageFmt,
		args...,
	)
}

func (h *SyncPowerStateHandler) recordStopEventf(ctx context.Context, obj client.Object, messageFmt string) {
	h.recorder.WithLogging(logger.FromContext(ctx)).Eventf(
		obj,
		corev1.EventTypeNormal,
		virtv2.ReasonVMStopped,
		messageFmt,
	)
}

func (h *SyncPowerStateHandler) recordRestartEventf(ctx context.Context, obj client.Object, messageFmt string) {
	h.recorder.WithLogging(logger.FromContext(ctx)).Eventf(
		obj,
		corev1.EventTypeNormal,
		virtv2.ReasonVMRestarted,
		messageFmt,
	)
}
