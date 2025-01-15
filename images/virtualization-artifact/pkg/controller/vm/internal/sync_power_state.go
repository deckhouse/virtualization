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

	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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

	switch runPolicy {
	case virtv2.AlwaysOffPolicy:
		if kvvmi != nil {
			h.recordStopEventf(ctx, s.VirtualMachine().Current(),
				"Stop initiated by controller to ensure %s policy",
				runPolicy,
			)

			// Ensure KVVMI is absent.
			err = h.client.Delete(ctx, kvvmi)
			if err != nil && !k8serrors.IsNotFound(err) {
				return fmt.Errorf("automatic stop VM for %s policy: delete KVVMI: %w", runPolicy, err)
			}
		}
	case virtv2.AlwaysOnPolicy:
		if kvvmi == nil {
			h.recordStartEventf(ctx, s.VirtualMachine().Current(),
				"Start initiated by controller for %v policy",
				runPolicy,
			)

			if err = powerstate.StartVM(ctx, h.client, kvvm); err != nil {
				return fmt.Errorf("failed to start VM: %w", err)
			}
		}

		if kvvmi != nil && kvvmi.DeletionTimestamp == nil {
			if kvvmi.Status.Phase == virtv1.Succeeded {
				if shutdownInfo.PodCompleted {
					// Treat completed Pod as restart if guest was restarted or as a start if guest was stopped.
					switch shutdownInfo.Reason {
					case powerstate.GuestResetReason:
						h.recordRestartEventf(ctx, s.VirtualMachine().Current(),
							"Restart initiated from inside the guest VM",
						)
					default:
						h.recordStartEventf(ctx, s.VirtualMachine().Current(),
							"Start initiated by controller after stopping from inside the guest VM",
						)
					}
				} else {
					h.recordRestartEventf(ctx, s.VirtualMachine().Current(),
						"Restart initiated by controller for %v runPolicy",
						runPolicy,
					)
				}
				err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
				if err != nil {
					return fmt.Errorf("restart VM on guest-reset: %w", err)
				}
			}

			if kvvmi.Status.Phase == virtv1.Failed {
				h.recordRestartEventf(ctx, s.VirtualMachine().Current(),
					"Restart initiated by controller for %s runPolicy after observing failed guest VM",
					runPolicy,
				)
				err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
				if err != nil {
					return fmt.Errorf("restart VM on failed: %w", err)
				}
			}
		}
	case virtv2.AlwaysOnUnlessStoppedManually:
		if kvvmi != nil && kvvmi.DeletionTimestamp == nil {
			if kvvmi.Status.Phase == virtv1.Succeeded {
				if shutdownInfo.PodCompleted {
					// Request to start new KVVMI if guest was restarted.
					// Cleanup KVVMI is enough if VM was stopped from inside.
					switch shutdownInfo.Reason {
					case powerstate.GuestResetReason:
						h.recordRestartEventf(ctx, s.VirtualMachine().Current(),
							"Restart initiated from inside the guest VM",
						)
						err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
						if err != nil {
							return fmt.Errorf("restart VM on guest-reset: %w", err)
						}
					default:
						vmPod, err := s.Pod(ctx)
						if err != nil {
							return fmt.Errorf("get virtual machine pod: %w", err)
						}

						if vmPod != nil && !vmPod.GetObjectMeta().GetDeletionTimestamp().IsZero() {
							h.recordRestartEventf(ctx, s.VirtualMachine().Current(),
								"Restart initiated by controller for %s runPolicy after the deletion of pod VM.",
								runPolicy,
							)
							err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
							if err != nil {
								return fmt.Errorf("automatic restart of failed VM: %w", err)
							}
						} else {
							h.recordStopEventf(ctx, s.VirtualMachine().Current(),
								"Stop initiated from inside the guest VM",
							)
							err = h.client.Delete(ctx, kvvmi)
							if err != nil && !k8serrors.IsNotFound(err) {
								return fmt.Errorf("delete Succeeded KVVMI: %w", err)
							}
						}
					}
				}
			}
			if kvvmi.Status.Phase == virtv1.Failed {
				h.recordRestartEventf(ctx, s.VirtualMachine().Current(),
					"Restart initiated by controller for %s runPolicy after observing failed guest VM",
					runPolicy,
				)
				err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
				if err != nil {
					return fmt.Errorf("automatic restart of failed VM: %w", err)
				}
			}
		}
	case virtv2.ManualPolicy:
		// Manual policy requires to handle only guest-reset event.
		// All types of shutdown are final states.
		if kvvmi != nil && kvvmi.DeletionTimestamp == nil {
			if kvvmi.Status.Phase == virtv1.Succeeded && shutdownInfo.PodCompleted {
				// Request to start new KVVMI (with updated settings).
				switch shutdownInfo.Reason {
				case powerstate.GuestResetReason:
					h.recordRestartEventf(ctx, s.VirtualMachine().Current(),
						"Restart initiated from inside the guest VM",
					)
					err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
					if err != nil {
						return fmt.Errorf("restart VM on guest-reset: %w", err)
					}
				default:
					h.recordStopEventf(ctx, s.VirtualMachine().Current(),
						"Stop initiated from inside the guest VM",
					)
					// Cleanup old version of KVVMI.
					err = h.client.Delete(ctx, kvvmi)
					if err != nil && !k8serrors.IsNotFound(err) {
						return fmt.Errorf("delete Succeeded KVVMI: %w", err)
					}
				}
			}
		}
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
