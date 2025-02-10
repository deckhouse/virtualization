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

	err = h.syncKVVMAnnotations(ctx, changed, kvvm)
	if err != nil {
		err = fmt.Errorf("failed to sync KVVM power state annotations: %w", err)
		log.Error(err.Error())
	}

	return reconcile.Result{}, err
}

func (h *SyncPowerStateHandler) syncKVVMAnnotations(ctx context.Context, vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) error {
	if vm.Status.Phase == virtv2.MachineStarting && kvvm.Annotations[annotations.AnnVmStartRequested] == "true" {
		err := removeStartAnnotationToKVVM(ctx, h.client, kvvm)
		if err != nil {
			return err
		}
	}

	return nil
}

// syncPowerState enforces runPolicy on the underlying KVVM.
func (h *SyncPowerStateHandler) syncPowerState(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, runPolicy virtv2.RunPolicy) error {
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
	canStartVM := appliedCondition.Status == metav1.ConditionTrue

	switch runPolicy {
	case virtv2.AlwaysOffPolicy:
		return h.handleAlwaysOffPolicy(ctx, s, kvvmi, runPolicy)
	case virtv2.AlwaysOnPolicy:
		return h.handleAlwaysOnPolicy(ctx, s, kvvm, kvvmi, canStartVM, runPolicy, shutdownInfo)
	case virtv2.AlwaysOnUnlessStoppedManually:
		return h.handleAlwaysOnUnlessStoppedManuallyPolicy(ctx, s, kvvm, kvvmi, canStartVM, runPolicy, shutdownInfo)
	case virtv2.ManualPolicy:
		return h.handleManualPolicy(ctx, s, kvvm, kvvmi, canStartVM, runPolicy, shutdownInfo)
	}

	return nil
}

func (h *SyncPowerStateHandler) handleAlwaysOffPolicy(ctx context.Context,
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

func (h *SyncPowerStateHandler) handleStartRequest(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, runPolicy virtv2.RunPolicy, canStartVM bool) error {
	if !canStartVM {
		return h.handleCanNotStartVM(ctx, s, kvvm, nil, runPolicy)
	}
	h.recordStartEventf(ctx, s.VirtualMachine().Current(), "Start initiated by controller for %v policy", runPolicy)
	return h.startVM(ctx, kvvm)
}

func (h *SyncPowerStateHandler) startVM(ctx context.Context, kvvm *virtv1.VirtualMachine) error {
	if err := powerstate.StartVM(ctx, h.client, kvvm); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}
	return nil
}

func (h *SyncPowerStateHandler) handleRestart(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, runPolicy virtv2.RunPolicy, canStartVM bool, reason string) error {
	if !canStartVM {
		return h.handleCanNotStartVM(ctx, s, kvvm, kvvmi, runPolicy)
	}
	h.recordRestartEventf(ctx, s.VirtualMachine().Current(), "Restart initiated by %s for %s runPolicy", reason, runPolicy)
	return h.safeRestartVM(ctx, kvvm, kvvmi)
}

func (h *SyncPowerStateHandler) safeRestartVM(ctx context.Context, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) error {
	if err := powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi); err != nil {
		return fmt.Errorf("restart VM: %w", err)
	}
	return nil
}

func (h *SyncPowerStateHandler) handleManualPolicy(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, canStartVM bool, runPolicy virtv2.RunPolicy, shutdownInfo powerstate.ShutdownInfo) error {
	if kvvmi != nil && kvvmi.DeletionTimestamp == nil {
		if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" {
			if kvvmi.Status.Phase == virtv1.Running {
				return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "VMOP")
			}
		} else if kvvmi.Status.Phase == virtv1.Succeeded && shutdownInfo.PodCompleted {
			if shutdownInfo.Reason == powerstate.GuestResetReason {
				return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "inside the guest VM")
			} else {
				h.recordStopEventf(ctx, s.VirtualMachine().Current(), "Stop initiated from inside the guest VM")
				return h.deleteSucceededKVVMI(ctx, kvvmi)
			}
		}
	} else if canStartVM && (kvvm.Annotations[annotations.AnnVmStartRequested] == "true" || kvvm.Annotations[annotations.AnnVmRestartRequested] == "true") {
		return h.handleStartRequest(ctx, s, kvvm, runPolicy, canStartVM)
	}
	return nil
}

func (h *SyncPowerStateHandler) handleAlwaysOnPolicy(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, canStartVM bool, runPolicy virtv2.RunPolicy, shutdownInfo powerstate.ShutdownInfo) error {
	if kvvmi == nil {
		return h.handleStartRequest(ctx, s, kvvm, runPolicy, canStartVM)
	}

	if kvvmi.DeletionTimestamp == nil {
		if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" && kvvmi.Status.Phase == virtv1.Running {
			return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "VMOP")
		}

		if kvvmi.Status.Phase == virtv1.Succeeded || kvvmi.Status.Phase == virtv1.Failed {
			if shutdownInfo.PodCompleted {
				if shutdownInfo.Reason == powerstate.GuestResetReason {
					return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "inside the guest VM")
				}
			}
			return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "controller after stopping from inside the guest VM")
		}
	} else if canStartVM && (kvvm.Annotations[annotations.AnnVmStartRequested] == "true" || kvvm.Annotations[annotations.AnnVmRestartRequested] == "true") {
		return h.handleStartRequest(ctx, s, kvvm, runPolicy, canStartVM)
	}
	return nil
}

func (h *SyncPowerStateHandler) handleAlwaysOnUnlessStoppedManuallyPolicy(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, canStartVM bool, runPolicy virtv2.RunPolicy, shutdownInfo powerstate.ShutdownInfo) error {
	if kvvmi != nil && kvvmi.DeletionTimestamp == nil {
		if kvvm.Annotations[annotations.AnnVmRestartRequested] == "true" && kvvmi.Status.Phase == virtv1.Running {
			return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "VMOP")
		}

		if kvvmi.Status.Phase == virtv1.Succeeded {
			if shutdownInfo.PodCompleted {
				if shutdownInfo.Reason == powerstate.GuestResetReason {
					return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "inside the guest VM")
				} else {
					vmPod, err := s.Pod(ctx)
					if err != nil {
						return fmt.Errorf("get virtual machine pod: %w", err)
					}
					if vmPod != nil && !vmPod.GetObjectMeta().GetDeletionTimestamp().IsZero() {
						return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "controller after the deletion of pod VM")
					}
					h.recordStopEventf(ctx, s.VirtualMachine().Current(), "Stop initiated from inside the guest VM")
					return h.deleteSucceededKVVMI(ctx, kvvmi)
				}
			}
		}
		if kvvmi.Status.Phase == virtv1.Failed {
			return h.handleRestart(ctx, s, kvvm, kvvmi, runPolicy, canStartVM, "controller after observing failed guest VM")
		}
	} else if canStartVM && (kvvm.Annotations[annotations.AnnVmStartRequested] == "true" || kvvm.Annotations[annotations.AnnVmRestartRequested] == "true") {
		return h.handleStartRequest(ctx, s, kvvm, runPolicy, canStartVM)
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

func (h *SyncPowerStateHandler) handleCanNotStartVM(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, runPolicy virtv2.RunPolicy) error {
	h.recordStopEventf(ctx, s.VirtualMachine().Current(),
		"The VirtualMachine failed to start because the provided configuration could not be applied.",
	)
	if kvvmi != nil {
		err := h.client.Delete(ctx, kvvmi)
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("stop VM for %s policy: delete KVVMI: %w", runPolicy, err)
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
