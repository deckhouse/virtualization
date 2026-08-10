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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
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

func NewHotplugHandler(client client.Client, migration OneShotMigration, inplaceResize *inplaceresize.Service, featureGate featuregate.FeatureGate, recorder eventrecord.EventRecorderLogger) *HotplugHandler {
	return &HotplugHandler{
		client:               client,
		oneShotMigration:     migration,
		inplaceResize:        inplaceResize,
		featureGate:          featureGate,
		recorder:             recorder,
		inplaceResizeTimeout: inplaceResizeConditionTimeout,
		inplaceResizeWaits:   make(map[types.NamespacedName]inplaceResizeWait),
	}
}

type HotplugHandler struct {
	client           client.Client
	oneShotMigration OneShotMigration
	inplaceResize    *inplaceresize.Service
	featureGate      featuregate.FeatureGate
	recorder         eventrecord.EventRecorderLogger

	inplaceResizeTimeout time.Duration

	mu                 sync.Mutex
	inplaceResizeWaits map[types.NamespacedName]inplaceResizeWait
}

// inplaceResizeWait remembers when the handler started waiting for the runtime to confirm the
// in-place resize of a particular desired state, and whether the fallback to live migration has
// already been announced for it.
type inplaceResizeWait struct {
	resourcesSum  string
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

	log := logger.FromContext(ctx).With(logger.SlogHandler(hotplugHandler))
	ctx = logger.ToContext(ctx, log)

	if !h.inplaceResize.InProgress(kvvmi) {
		h.forgetInplaceResizeWait(vm)
	} else {
		completed := h.inplaceResize.IsCompleted(kvvmi)
		possible, err := h.inplaceResize.IsPossible(ctx, kvvmi)
		switch {
		case err == nil:
			if possible || completed {
				h.forgetInplaceResizeWait(vm)
				return reconcile.Result{}, nil
			}
			// inplace resize is not possible, but it is not complete
			// switch to resize via live migration
		case errors.Is(err, inplaceresize.ErrConditionNotFound):
			// The runtime has not confirmed the resize yet. Give it a bounded amount of time:
			// the condition never appears for a rejected resize, so waiting indefinitely
			// deadlocks the virtual machine instead of falling back to live migration.
			elapsed := h.noteInplaceResizeWait(vm, getHotplugResourcesSum(vm))
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

	migrate, err := h.oneShotMigration.OnceMigrate(ctx, vm, annotations.AnnVMOPWorkloadUpdateHotplugResourcesSum, getHotplugResourcesSum(vm))
	if migrate {
		log.Info("The virtual machine was triggered to migrate by the hotplug resources handler.")
	}

	return reconcile.Result{}, err
}

func (h *HotplugHandler) Name() string {
	return hotplugHandler
}

// noteInplaceResizeWait reports how long the handler has been waiting for the runtime to confirm
// the in-place resize of resourcesSum, starting the countdown on the first call. Zero means the
// wait has just started.
//
// The countdown is kept per desired state: a change of resourcesSum restarts it, so a fresh
// request always gets the full timeout even when the previous one ended in a fallback. Without
// that, every subsequent change would be migrated immediately instead of being given a chance to
// be applied in place.
func (h *HotplugHandler) noteInplaceResizeWait(vm *v1alpha2.VirtualMachine, resourcesSum string) time.Duration {
	key := object.NamespacedName(vm)

	h.mu.Lock()
	defer h.mu.Unlock()

	wait, found := h.inplaceResizeWaits[key]
	if !found || wait.resourcesSum != resourcesSum {
		h.inplaceResizeWaits[key] = inplaceResizeWait{resourcesSum: resourcesSum, since: time.Now()}
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

func isAwaitingRestartToApplyConfiguration(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}

func getHotplugResourcesSum(vm *v1alpha2.VirtualMachine) string {
	return fmt.Sprintf("cpu.cores=%d,cpu.coreFraction=%s,memory.size=%s", vm.Spec.CPU.Cores, vm.Spec.CPU.CoreFraction, vm.Spec.Memory.Size.String())
}
