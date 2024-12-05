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
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameSyncPowerStateHandler = "SyncPowerStateHandler"

func NewSyncPowerStateHandler(client client.Client, recorder record.EventRecorder) *SyncPowerStateHandler {
	return &SyncPowerStateHandler{
		client:   client,
		recorder: recorder,
	}
}

type SyncPowerStateHandler struct {
	client   client.Client
	recorder record.EventRecorder
}

func (h *SyncPowerStateHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	cbConfApplied := conditions.NewConditionBuilder(vmcondition.TypeConfigurationApplied).
		Generation(current.GetGeneration()).
		Status(metav1.ConditionUnknown).
		Reason(conditions.ReasonUnknown)

	defer func() {
		conditions.SetCondition(cbConfApplied, &changed.Status.Conditions)
	}()

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, err
	}

	err = h.syncPowerState(ctx, s, kvvm, &changed.Spec)
	if err != nil {
		err = fmt.Errorf("failed to sync powerstate: %w", err)
		h.recorder.Event(current, corev1.EventTypeWarning, virtv2.ReasonErrVmNotSynced, err.Error())
		cbConfApplied.
			Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonConfigurationNotApplied).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
	}

	return reconcile.Result{}, err
}

// syncPowerState enforces runPolicy on the underlying KVVM.
func (h *SyncPowerStateHandler) syncPowerState(ctx context.Context, s state.VirtualMachineState, kvvm *virtv1.VirtualMachine, effectiveSpec *virtv2.VirtualMachineSpec) error {
	log := logger.FromContext(ctx)

	if kvvm == nil {
		return nil
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return fmt.Errorf("find the internal virtual machine instance: %w", err)
	}

	vmRunPolicy := effectiveSpec.RunPolicy
	var shutdownInfo powerstate.ShutdownInfo
	s.Shared(func(s *state.Shared) {
		shutdownInfo = s.ShutdownInfo
	})

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
		strategy, _ := kvvm.RunStrategy()
		if strategy == virtv1.RunStrategyAlways && kvvmi == nil {
			if err = powerstate.StartVM(ctx, h.client, kvvm); err != nil {
				return fmt.Errorf("failed to start VM: %w", err)
			}
		}

		if kvvmi != nil && kvvmi.DeletionTimestamp == nil {
			if kvvmi.Status.Phase == virtv1.Succeeded {
				log.Info("Restart for guest initiated reset")
				err = powerstate.SafeRestartVM(ctx, h.client, kvvm, kvvmi)
				if err != nil {
					return fmt.Errorf("restart VM on guest-reset: %w", err)
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
