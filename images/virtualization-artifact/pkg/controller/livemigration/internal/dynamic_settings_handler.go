/*
Copyright 2025 Flant JSC

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

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/livemigration"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const dynamicSettingsHandlerName = "DynamicSettingsHandler"

func NewDynamicSettingsHandler(client client.Client, liveMigrationSettings config.LiveMigrationSettings) *DynamicSettingsHandler {
	return &DynamicSettingsHandler{
		client:         client,
		moduleSettings: liveMigrationSettings,
	}
}

type DynamicSettingsHandler struct {
	client         client.Client
	moduleSettings config.LiveMigrationSettings
}

func (h *DynamicSettingsHandler) Handle(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(dynamicSettingsHandlerName))

	if !h.shouldUpdateMigrationConfiguration(kvvmi) {
		return reconcile.Result{}, nil
	}

	var vm v1alpha2.VirtualMachine
	vmKey := client.ObjectKeyFromObject(kvvmi)
	err := h.client.Get(ctx, vmKey, &vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	// TODO Temporary fix to have liveMigration settings in one place.
	// TODO Remove after implementing liveMigration settings.
	var kvconfig virtv1.KubeVirt
	configKey := types.NamespacedName{
		Namespace: "d8-virtualization",
		Name:      "config",
	}
	err = h.client.Get(ctx, configKey, &kvconfig)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetch InProgress vmop
	vmopInProgress, err := h.getVMOPInProgressForVM(ctx, vmKey)
	if err != nil {
		return reconcile.Result{}, err
	}

	effectivePolicy, autoConverge, err := livemigration.CalculateEffectivePolicy(vm, vmopInProgress)
	if err != nil {
		return reconcile.Result{}, err
	}

	conf := livemigration.NewMigrationConfiguration(h.moduleSettings, autoConverge, kvconfig)

	kvvmi.Status.MigrationState.MigrationConfiguration = conf

	log.Debug("Set migrationConfiguration on KVVMI",
		"migrationConfiguration", livemigration.DumpKVVMIMigrationConfiguration(kvvmi),
		"policy", effectivePolicy,
		"autoConverge", autoConverge,
	)

	return reconcile.Result{}, nil
}

func (h *DynamicSettingsHandler) Name() string {
	return dynamicSettingsHandlerName
}

// shouldUpdateMigrationConfiguration indicates if live migration controller should inject
// migration configuration into KVVMI status:
// 1. status.migrationState is created by the virt-controller.
// 2. migration is not in a Completed state.
func (h *DynamicSettingsHandler) shouldUpdateMigrationConfiguration(kvvmi *virtv1.VirtualMachineInstance) bool {
	return kvvmi.Status.MigrationState != nil &&
		!kvvmi.Status.MigrationState.Completed
}

// getVMOPInProgressForVM check if there is at least one VMOP for the same VM in progress phase.
func (h *DynamicSettingsHandler) getVMOPInProgressForVM(ctx context.Context, vmKey client.ObjectKey) (*v1alpha2.VirtualMachineOperation, error) {
	var vmopList v1alpha2.VirtualMachineOperationList
	err := h.client.List(ctx, &vmopList, client.InNamespace(vmKey.Namespace))
	if err != nil {
		return nil, err
	}

	for _, vmop := range vmopList.Items {
		// Ignore VMOPs for other VMs.
		if vmop.Spec.VirtualMachine != vmKey.Name {
			continue
		}

		// Return if VMOP has phase InProgress.
		if vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress {
			return &vmop, nil
		}
	}
	return nil, nil
}
