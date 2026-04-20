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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func NewHotplugHandler(client client.Client, migration OneShotMigration, inplaceResize *inplaceresize.Service, featureGate featuregate.FeatureGate, recorder eventrecord.EventRecorderLogger) *HotplugHandler {
	return &HotplugHandler{
		client:           client,
		oneShotMigration: migration,
		inplaceResize:    inplaceResize,
		featureGate:      featureGate,
		recorder:         recorder,
	}
}

type HotplugHandler struct {
	client           client.Client
	oneShotMigration OneShotMigration
	inplaceResize    *inplaceresize.Service
	featureGate      featuregate.FeatureGate
	recorder         eventrecord.EventRecorderLogger
}

func (h *HotplugHandler) Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	if vm == nil || !vm.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	if isAwaitingRestartToApplyConfiguration(vm) {
		return reconcile.Result{}, nil
	}

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := h.client.Get(ctx, object.NamespacedName(vm), kvvmi); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if h.inplaceResize.InProgress(kvvmi) {
		completed := h.inplaceResize.IsCompleted(kvvmi)
		possible, err := h.inplaceResize.IsPossible(ctx, kvvmi)
		if err != nil {
			if errors.Is(err, inplaceresize.ErrConditionNotFound) {
				logger.FromContext(ctx).Info("Waiting for inplace resize condition, requeue after 1 second")
				return reconcile.Result{RequeueAfter: 1 * time.Second}, nil
			}
			return reconcile.Result{}, err
		}

		if possible || completed {
			return reconcile.Result{}, nil
		}
		// inplace resize is not possible, but it is not complete
		// switch to resize via live migration
	}

	cond, _ := conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceMemoryChange, kvvmi.Status.Conditions)
	isMemoryHotplug := cond.Status == corev1.ConditionTrue

	cond, _ = conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceVCPUChange, kvvmi.Status.Conditions)
	isCPUHotplug := cond.Status == corev1.ConditionTrue

	if !isCPUHotplug && !isMemoryHotplug {
		return reconcile.Result{}, nil
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(hotplugHandler))
	ctx = logger.ToContext(ctx, log)

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

func isAwaitingRestartToApplyConfiguration(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}

func getHotplugResourcesSum(vm *v1alpha2.VirtualMachine) string {
	return fmt.Sprintf("cpu.cores=%d,cpu.coreFraction=%s,memory.size=%s", vm.Spec.CPU.Cores, vm.Spec.CPU.CoreFraction, vm.Spec.Memory.Size.String())
}
