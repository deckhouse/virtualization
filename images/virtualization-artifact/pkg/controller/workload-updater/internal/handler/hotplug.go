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

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const hotplugHandler = "HotplugHandler"

func NewHotplugHandler(client client.Client, migration OneShotMigration) *HotplugHandler {
	return &HotplugHandler{
		client:           client,
		oneShotMigration: migration,
	}
}

type HotplugHandler struct {
	client           client.Client
	oneShotMigration OneShotMigration
}

func (h *HotplugHandler) Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	if vm == nil || !vm.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := h.client.Get(ctx, object.NamespacedName(vm), kvvmi); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	cond, _ := conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceMemoryChange, kvvmi.Status.Conditions)
	if cond.Status != corev1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(hotplugHandler))
	ctx = logger.ToContext(ctx, log)

	migrate, err := h.oneShotMigration.OnceMigrate(ctx, vm, annotations.AnnVMOPWorkloadUpdateHotplugResourcesSum, getHotplugResourcesSum(vm))
	if migrate {
		log.Info("The virtual machine was triggered to migrate by the hotplug resources handler.")
	}

	return reconcile.Result{}, err
}

func (h *HotplugHandler) Name() string {
	return hotplugHandler
}

func getHotplugResourcesSum(vm *v1alpha2.VirtualMachine) string {
	return vm.Spec.Memory.Size.String()
}
