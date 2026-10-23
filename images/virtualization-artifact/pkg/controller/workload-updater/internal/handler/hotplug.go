/*
Copyright 2026 Flant JSC

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

package handler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/hotplugmigration"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/inplaceresize"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const hotplugHandler = "HotplugHandler"

// inplaceResizeConditionTimeout bounds the wait for the PodResourceResizeInProgress condition
// on the KVVMI. The condition appears only for a resize the runtime has accepted: a request
// rejected by the apiserver (decreasing memory limits, changing the pod QoS class) or by the
// kubelet (a Guaranteed pod on a node with the static CPU manager policy) never gets it.
// Without a bound the virtual machine would stay in "hotplug in progress" forever and would
// never fall back to resize via live migration.
const inplaceResizeConditionTimeout = 30 * time.Second

// hotplugMigrationRetryInterval bounds the wait for the hotplug migration to be created when
// another operation on the virtual machine holds it back.
const hotplugMigrationRetryInterval = 30 * time.Second

func NewHotplugHandler(client client.Client, migration HotplugMigration, inplaceResize *inplaceresize.Service, featureGate featuregate.FeatureGate, recorder eventrecord.EventRecorderLogger) *HotplugHandler {
	return &HotplugHandler{
		client:               client,
		migration:            migration,
		inplaceResize:        inplaceResize,
		featureGate:          featureGate,
		recorder:             recorder,
		inplaceResizeTimeout: inplaceResizeConditionTimeout,
		inplaceResizeWaits:   make(map[types.NamespacedName]inplaceResizeWait),
	}
}

type HotplugHandler struct {
	client        client.Client
	migration     HotplugMigration
	inplaceResize *inplaceresize.Service
	featureGate   featuregate.FeatureGate
	recorder      eventrecord.EventRecorderLogger

	inplaceResizeTimeout time.Duration

	mu                 sync.Mutex
	inplaceResizeWaits map[types.NamespacedName]inplaceResizeWait
}

// inplaceResizeWait remembers when the handler started waiting for the runtime to confirm the
// in-place resize of a particular desired state, and whether the fallback to live migration has
// already been announced for it.
type inplaceResizeWait struct {
	configKey     string
	since         time.Time
	fallbackNoted bool
}

func (h *HotplugHandler) Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	if vm == nil {
		return reconcile.Result{}, nil
	}

	if !vm.GetDeletionTimestamp().IsZero() {
		h.forgetInplaceResizeWait(vm)
		return reconcile.Result{}, nil
	}

	if isAwaitingRestartToApplyConfiguration(vm) {
		return reconcile.Result{}, nil
	}

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := h.client.Get(ctx, object.NamespacedName(vm), kvvmi); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// A stopped KVVMI keeps the resize and hotplug conditions of its last pod, but nothing applies
	// them any more: neither a migration nor the wait for the runtime makes sense.
	if kvvmi.IsFinal() || !kvvmi.GetDeletionTimestamp().IsZero() {
		h.forgetInplaceResizeWait(vm)
		return reconcile.Result{}, nil
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(hotplugHandler))
	ctx = logger.ToContext(ctx, log)

	pod, err := kvvm.FindPodByKVVMI(ctx, h.client, kvvmi)
	if err != nil {
		return reconcile.Result{}, err
	}
	vmops, err := h.listMigrations(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Recorded before the in-place path returns: otherwise a return to the configuration a migration
	// produced the pod for looks like a migration that did not apply it.
	var plan hotplugmigration.Plan
	if pod != nil {
		plan = hotplugmigration.Evaluate(hotplugmigration.Input{Now: time.Now(), VM: vm, KVVMI: kvvmi, Pod: pod, VMOPs: vmops})
		if plan.RecordMark {
			if err = object.EnsureAnnotation(ctx, h.client, kvvmi, hotplugmigration.Annotation, plan.Mark); err != nil {
				return reconcile.Result{}, fmt.Errorf("record the configuration migrated into the launcher pod: %w", err)
			}
		}
	}

	if !h.inplaceResize.InProgress(kvvmi) {
		h.forgetInplaceResizeWait(vm)
	} else {
		completed := h.inplaceResize.IsCompleted(kvvmi)
		possible, err := h.inplaceResize.IsPossible(ctx, kvvmi)
		switch {
		case err == nil && possible:
			h.forgetInplaceResizeWait(vm)
			return reconcile.Result{}, nil
		case err == nil && completed:
			// A resize condition left over from the pod before a live migration reports the current pod
			// as resized while it is not: only another migration applies the changes then. The pending
			// conditions flicker in that state, so the wait outlives them.
			if !hasPendingHotplug(kvvmi) {
				return reconcile.Result{}, nil
			}
			elapsed := h.noteInplaceResizeWait(vm, hotplugmigration.Key(kvvmi))
			if elapsed < h.inplaceResizeTimeout {
				return reconcile.Result{RequeueAfter: 1 * time.Second}, nil
			}
			if h.noteInplaceResizeFallback(vm) {
				log.Info("The runtime has not applied the changes after the inplace resize, switch to resize via live migration",
					"timeout", h.inplaceResizeTimeout.String())
				h.recorder.WithLogging(log).Event(vm, corev1.EventTypeNormal, v1alpha2.ReasonVMCPUResizing,
					"In-place resize has completed, but the runtime has not applied the changes; applying them via live migration.")
			}
		case err == nil:
			// inplace resize is not possible, but it is not complete
			// switch to resize via live migration
		case errors.Is(err, inplaceresize.ErrConditionNotFound):
			// The runtime has not confirmed the resize yet. Give it a bounded amount of time:
			// the condition never appears for a rejected resize, so waiting indefinitely
			// deadlocks the virtual machine instead of falling back to live migration.
			elapsed := h.noteInplaceResizeWait(vm, hotplugmigration.Key(kvvmi))
			if elapsed < h.inplaceResizeTimeout {
				if elapsed == 0 {
					log.Info("Waiting for the runtime to confirm the inplace resize",
						"timeout", h.inplaceResizeTimeout.String())
				} else {
					log.Debug("Waiting for inplace resize condition, requeue after 1 second")
				}
				return reconcile.Result{RequeueAfter: 1 * time.Second}, nil
			}

			// The fallback is re-evaluated on every reconcile until the migration applies the
			// changes, so announce it only once per desired state.
			if h.noteInplaceResizeFallback(vm) {
				log.Info("Inplace resize is not confirmed by the runtime, switch to resize via live migration",
					"timeout", h.inplaceResizeTimeout.String())
				h.recorder.WithLogging(log).Event(vm, corev1.EventTypeNormal, v1alpha2.ReasonVMCPUResizing,
					"In-place resize has not been confirmed by the runtime, applying the changes via live migration.")
			}
		default:
			return reconcile.Result{}, err
		}
	}

	cond, _ := conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceMemoryChange, kvvmi.Status.Conditions)
	isMemoryHotplug := cond.Status == corev1.ConditionTrue

	cond, _ = conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceVCPUChange, kvvmi.Status.Conditions)
	isCPUHotplug := cond.Status == corev1.ConditionTrue

	if !isCPUHotplug && !isMemoryHotplug {
		return reconcile.Result{}, nil
	}

	if isMemoryHotplug && !h.featureGate.Enabled(featuregates.HotplugMemoryWithLiveMigration) {
		h.recorder.WithLogging(log).Event(vm, corev1.EventTypeWarning, v1alpha2.ReasonVMHotplugMemoryNotSupported, "HotplugMemoryWithLiveMigration feature gate is not enabled")
		return reconcile.Result{}, nil
	}

	if isCPUHotplug && !h.featureGate.Enabled(featuregates.HotplugCPUWithLiveMigration) {
		h.recorder.WithLogging(log).Event(vm, corev1.EventTypeWarning, v1alpha2.ReasonVMHotplugCPUNotSupported, "HotplugCPUWithLiveMigration feature gate is not enabled")
		return reconcile.Result{}, nil
	}

	if pod == nil {
		return reconcile.Result{}, nil
	}

	switch plan.Phase {
	case hotplugmigration.PhaseDue:
	case hotplugmigration.PhaseSettling, hotplugmigration.PhaseRetrying:
		return reconcile.Result{RequeueAfter: max(time.Until(plan.At), time.Second)}, nil
	default:
		// A running migration re-queues the virtual machine when it ends; a configuration that a
		// migration did not apply or that ran out of attempts waits for the next change.
		return reconcile.Result{}, nil
	}

	// A virtual machine that cannot move keeps the change pending on its node, which applies it
	// once it has room.
	if !isLiveMigratable(kvvmi) {
		log.Debug("The virtual machine cannot be migrated to apply the hotplug resources.")
		return reconcile.Result{}, nil
	}

	migrate, err := h.migration.Migrate(ctx, vm, hotplugmigration.Annotation, plan.Key)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !migrate {
		return reconcile.Result{RequeueAfter: hotplugMigrationRetryInterval}, nil
	}

	log.Info("The virtual machine was triggered to migrate by the hotplug resources handler.")
	return reconcile.Result{}, nil
}

func (h *HotplugHandler) listMigrations(ctx context.Context, vm *v1alpha2.VirtualMachine) ([]*v1alpha2.VirtualMachineOperation, error) {
	vmopList := &v1alpha2.VirtualMachineOperationList{}
	if err := h.client.List(ctx, vmopList, client.InNamespace(vm.GetNamespace()),
		client.MatchingFields{indexer.IndexFieldVMOPByVM: vm.GetName()}); err != nil {
		return nil, fmt.Errorf("list virtual machine operations: %w", err)
	}
	var vmops []*v1alpha2.VirtualMachineOperation
	for i := range vmopList.Items {
		if commonvmop.IsMigration(&vmopList.Items[i]) {
			vmops = append(vmops, &vmopList.Items[i])
		}
	}
	return vmops, nil
}

func (h *HotplugHandler) Name() string {
	return hotplugHandler
}

// noteInplaceResizeWait reports how long the handler has been waiting for the runtime to confirm
// the in-place resize of configKey, starting the countdown on the first call. Zero means the
// wait has just started.
//
// The countdown is kept per desired state: a change of configKey restarts it, so a fresh
// request always gets the full timeout even when the previous one ended in a fallback. Without
// that, every subsequent change would be migrated immediately instead of being given a chance to
// be applied in place.
func (h *HotplugHandler) noteInplaceResizeWait(vm *v1alpha2.VirtualMachine, configKey string) time.Duration {
	key := object.NamespacedName(vm)

	h.mu.Lock()
	defer h.mu.Unlock()

	wait, found := h.inplaceResizeWaits[key]
	if !found || wait.configKey != configKey {
		h.inplaceResizeWaits[key] = inplaceResizeWait{configKey: configKey, since: time.Now()}
		return 0
	}

	return time.Since(wait.since)
}

// noteInplaceResizeFallback reports whether the fallback to live migration still has to be
// announced for the desired state being waited on. It returns true only on the first call, so
// repeated reconciles neither flood the log nor pile up events on the virtual machine.
func (h *HotplugHandler) noteInplaceResizeFallback(vm *v1alpha2.VirtualMachine) bool {
	key := object.NamespacedName(vm)

	h.mu.Lock()
	defer h.mu.Unlock()

	wait, found := h.inplaceResizeWaits[key]
	if found && wait.fallbackNoted {
		return false
	}

	wait.fallbackNoted = true
	h.inplaceResizeWaits[key] = wait

	return true
}

// forgetInplaceResizeWait stops tracking the wait for the virtual machine.
func (h *HotplugHandler) forgetInplaceResizeWait(vm *v1alpha2.VirtualMachine) {
	key := object.NamespacedName(vm)

	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.inplaceResizeWaits, key)
}

// hasPendingHotplug reports whether the runtime still has a CPU or memory change to apply.
func hasPendingHotplug(kvvmi *virtv1.VirtualMachineInstance) bool {
	for _, t := range []virtv1.VirtualMachineInstanceConditionType{virtv1.VirtualMachineInstanceMemoryChange, virtv1.VirtualMachineInstanceVCPUChange} {
		if cond, _ := conditions.GetKVVMICondition(t, kvvmi.Status.Conditions); cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isAwaitingRestartToApplyConfiguration(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}
