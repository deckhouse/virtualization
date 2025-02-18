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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameSyncPowerStateHandler = "SyncPowerStateHandler"

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

	appliedCondition, _ := conditions.GetCondition(vmcondition.TypeConfigurationApplied, s.VirtualMachine().Changed().Status.Conditions)
	isConfigurationApplied := appliedCondition.Status == metav1.ConditionTrue

	switch runPolicy {
	case virtv2.AlwaysOffPolicy:
		return h.handleAlwaysOffPolicy(ctx, s, kvvmi, runPolicy)
	case virtv2.AlwaysOnPolicy:
		return h.handleAlwaysOnPolicy(ctx, s, kvvm, kvvmi, isConfigurationApplied, runPolicy, shutdownInfo)
	case virtv2.AlwaysOnUnlessStoppedManually:
		return h.handleAlwaysOnUnlessStoppedManuallyPolicy(ctx, s, kvvm, kvvmi, isConfigurationApplied, runPolicy, shutdownInfo)
	case virtv2.ManualPolicy:
		return h.handleManualPolicy(ctx, s, kvvm, kvvmi, isConfigurationApplied, runPolicy, shutdownInfo)
	}

	return nil
}

func (h *SyncPowerStateHandler) handleAlwaysOffPolicy(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvmi *virtv1.VirtualMachineInstance,
	runPolicy virtv2.RunPolicy,
) error {
	if kvvmi != nil {
		h.recordStopEventf(ctx, s.VirtualMachine().Current(),
			"Stop initiated by controller to ensure %s policy",
			runPolicy,
		)

		err := h.client.Delete(ctx, kvvmi)
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("automatic stop VM for %s policy: delete KVVMI: %w", runPolicy, err)
		}
	}

	return nil
}

func (h *SyncPowerStateHandler) handleManualPolicy(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	isConfigurationApplied bool,
	runPolicy virtv2.RunPolicy,
	shutdownInfo powerstate.ShutdownInfo,
) error {
	if kvvmi == nil || kvvmi.DeletionTimestamp != nil {
		return h.checkNeedStartVM(ctx, s, kvvm, isConfigurationApplied, runPolicy)
	}

	if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" && kvvmi.Status.Phase == virtv1.Running {
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by VirtualMachineOparation for %s runPolicy", runPolicy)
		err := h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
		if err != nil {
			return err
		}

		err = kvvmutil.RemoveRestartAnnotation(ctx, h.client, kvvm)
		if err != nil {
			return err
		}

		return nil
	} else if kvvmi.Status.Phase == virtv1.Succeeded && shutdownInfo.PodCompleted {
		if shutdownInfo.Reason == powerstate.GuestResetReason {
			h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by inside the guest VirtualMachine for %s runPolicy", runPolicy)
			return h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
		} else {
			h.recordStopEventf(ctx, s.VirtualMachine().Current(), "Stop initiated from inside the guest VirtualMachine")
			return h.deleteSucceededKVVMI(ctx, kvvmi)
		}
	}

	return nil
}

func (h *SyncPowerStateHandler) handleAlwaysOnPolicy(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	isConfigurationApplied bool,
	runPolicy virtv2.RunPolicy,
	shutdownInfo powerstate.ShutdownInfo,
) error {
	if kvvmi == nil {
		h.recordStartEventf(ctx, s.VirtualMachine().Current(), "Start initiated by controller for %v policy", runPolicy)
		return h.start(ctx, s, kvvm, isConfigurationApplied)
	}

	if kvvmi.DeletionTimestamp != nil {
		return h.checkNeedStartVM(ctx, s, kvvm, isConfigurationApplied, runPolicy)
	}

	if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" && kvvmi.Status.Phase == virtv1.Running {
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by VirtualMachineOparation for %s runPolicy", runPolicy)
		err := h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
		if err != nil {
			return err
		}

		err = kvvmutil.RemoveRestartAnnotation(ctx, h.client, kvvm)
		if err != nil {
			return err
		}

		return nil
	}

	if kvvmi.Status.Phase == virtv1.Succeeded || kvvmi.Status.Phase == virtv1.Failed {
		if shutdownInfo.PodCompleted {
			if shutdownInfo.Reason == powerstate.GuestResetReason {
				h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by inside the guest VirtualMachine for %s runPolicy", runPolicy)
				return h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
			}
		}
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by controller after stopping from inside the guest VirtualMachine for %s runPolicy", runPolicy)
		return h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
	}

	return nil
}

func (h *SyncPowerStateHandler) handleAlwaysOnUnlessStoppedManuallyPolicy(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	kvvmi *virtv1.VirtualMachineInstance,
	isConfigurationApplied bool,
	runPolicy virtv2.RunPolicy,
	shutdownInfo powerstate.ShutdownInfo,
) error {
	if kvvmi == nil || kvvmi.DeletionTimestamp != nil {
		return h.checkNeedStartVM(ctx, s, kvvm, isConfigurationApplied, runPolicy)
	}

	if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" && kvvmi.Status.Phase == virtv1.Running {
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by VirtualMachineOparation for %s runPolicy", runPolicy)
		err := h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
		if err != nil {
			return err
		}

		err = kvvmutil.RemoveRestartAnnotation(ctx, h.client, kvvm)
		if err != nil {
			return err
		}

		return nil
	}
	switch kvvmi.Status.Phase {
	case virtv1.Succeeded:
		vmPod, err := s.Pod(ctx)
		if err != nil {
			return fmt.Errorf("get virtual machine pod: %w", err)
		}

		if shutdownInfo.PodCompleted {
			if shutdownInfo.Reason == powerstate.GuestResetReason {
				h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by inside the guest VirtualMachine for %s runPolicy", runPolicy)
				return h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
			} else {
				if vmPod == nil || !vmPod.GetObjectMeta().GetDeletionTimestamp().IsZero() {
					h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by controller after the deletion of pod VirtualMachine for %s runPolicy", runPolicy)
					return h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
				}
				h.recordStopEventf(ctx, s.VirtualMachine().Current(), "Stop initiated from inside the guest VirtualMachine")
				return h.deleteSucceededKVVMI(ctx, kvvmi)
			}
		}

		if vmPod == nil {
			log, _ := logger.GetHandlerContext(ctx, nameSyncPowerStateHandler)
			log.Error("failed to find VM pod")
		}
	case virtv1.Failed:
		h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by controller after observing failed guest VirtualMachine for %s runPolicy", runPolicy)
		return h.restart(ctx, s, kvvm, kvvmi, isConfigurationApplied)
	default:
		return nil
	}

	return nil
}

func (h *SyncPowerStateHandler) checkNeedStartVM(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	isConfigurationApplied bool,
	runPolicy virtv2.RunPolicy,
) error {
	if isConfigurationApplied && (kvvm.Annotations[annotations.AnnVmStartRequested] == "true" || kvvm.Annotations[annotations.AnnVmRestartRequested] == "true") {
		h.recordStartEventf(ctx, s.VirtualMachine().Current(), "Start initiated by controller for %v policy", runPolicy)
		err := h.start(ctx, s, kvvm, isConfigurationApplied)
		if err != nil {
			return err
		}

		err = kvvmutil.RemoveStartAnnotation(ctx, h.client, kvvm)
		if err != nil {
			return err
		}

		err = kvvmutil.RemoveRestartAnnotation(ctx, h.client, kvvm)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *SyncPowerStateHandler) start(
	ctx context.Context,
	s state.VirtualMachineState,
	kvvm *virtv1.VirtualMachine,
	isConfigurationApplied bool,
) error {
	if !isConfigurationApplied {
		h.recordStopEventf(ctx, s.VirtualMachine().Current(),
			"The VirtualMachine has been interrupted because the provided configuration could not be applied.",
		)
		return h.interruptRunningVM(ctx, kvvm, nil)
	}

	if err := powerstate.StartVM(ctx, h.client, kvvm); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}
	return nil
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
			"The VirtualMachine has been interrupted because the provided configuration could not be applied.",
		)
		return h.interruptRunningVM(ctx, kvvm, kvvmi)
	}

	if err := powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi); err != nil {
		return fmt.Errorf("restart VM: %w", err)
	}
	return nil
}

func (h *SyncPowerStateHandler) deleteSucceededKVVMI(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) error {
	err := h.client.Delete(ctx, kvvmi)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete Succeeded KVVMI: %w", err)
	}
	return nil
}

func (h *SyncPowerStateHandler) interruptRunningVM(ctx context.Context, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) error {
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

func (h *SyncPowerStateHandler) ensureRunStrategy(ctx context.Context, kvvm *virtv1.VirtualMachine, desiredRunStrategy virtv1.VirtualMachineRunStrategy) error {
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

func (h *SyncPowerStateHandler) recordStopEventf(ctx context.Context, obj client.Object, messageFmt string, args ...any) {
	h.recorder.WithLogging(logger.FromContext(ctx)).Eventf(
		obj,
		corev1.EventTypeNormal,
		virtv2.ReasonVMStopped,
		messageFmt,
		args...,
	)
}

func (h *SyncPowerStateHandler) recordRestartEventf(ctx context.Context, obj client.Object, messageFmt string, args ...any) {
	h.recorder.WithLogging(logger.FromContext(ctx)).Eventf(
		obj,
		corev1.EventTypeNormal,
		virtv2.ReasonVMRestarted,
		messageFmt,
		args...,
	)
}
