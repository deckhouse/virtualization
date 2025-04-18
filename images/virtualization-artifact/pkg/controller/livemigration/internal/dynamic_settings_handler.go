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
	internalservice "github.com/deckhouse/virtualization-controller/pkg/controller/livemigration/internal/service"
	service "github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const dynamicSettingsHandlerName = "DynamicSettingsHandler"

func NewDynamicSettingsHandler(client client.Client, liveMigrationSettings config.LiveMigrationSettings, vmopService service.VMOperationService) *DynamicSettingsHandler {
	return &DynamicSettingsHandler{
		client:         client,
		moduleSettings: liveMigrationSettings,
		vmopService:    vmopService,
	}
}

type DynamicSettingsHandler struct {
	client         client.Client
	moduleSettings config.LiveMigrationSettings
	vmopService    service.VMOperationService
}

func (h *DynamicSettingsHandler) Handle(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(dynamicSettingsHandlerName))

	if !h.shouldUpdateMigrationConfiguration(kvvmi) {
		return reconcile.Result{}, nil
	}

	// Get vm
	// Get vmop
	// Merge live migration policies.
	// Add global live migration settings.
	// Patch vmi status with migrationConfiguration.

	var vm virtv2.VirtualMachine
	vmKey := types.NamespacedName{
		Namespace: kvvmi.Namespace,
		Name:      kvvmi.Name,
	}
	err := h.client.Get(ctx, vmKey, &vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetch InProgress vmop
	vmopInProgress, err := h.vmopService.GetVMOPInProgressForVM(ctx, vmKey)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Merge
	// Detect policy: use default from vmclass if no policy on vm or policy is not allowed.
	// Use policy from vm if contains in allowed.
	// Set initial autoconverge value depending on policy.
	// Use force flag from vmop to override autoconverge flag for Manual, PreferSafe. Can't implement default force:true for PreferForced, do not override for now.

	vmPolicy := virtv2.PreferSafeMigrationPolicy
	if vm.Spec.LiveMigrationPolicy != "" {
		vmPolicy = vm.Spec.LiveMigrationPolicy
	}

	// Calculate final autoConverge value.
	autoConverge := false
	switch vmPolicy {
	case virtv2.PreferSafeMigrationPolicy:
		// User may override autoConverge with vmop.
		if vmopInProgress.Spec.Force != nil {
			autoConverge = *vmopInProgress.Spec.Force
		}
	case virtv2.PreferForcedMigrationPolicy:
		autoConverge = true
		if vmopInProgress.Spec.Force != nil {
			autoConverge = *vmopInProgress.Spec.Force
		}
	case virtv2.AlwaysForcedMigrationPolicy:
		autoConverge = true
	}

	conf := internalservice.NewMigrationConfiguration(h.moduleSettings, autoConverge)

	kvvmi.Status.MigrationState.MigrationConfiguration = conf

	log.Info("Patch KVVMI with migration settings", "migration.configuration", internalservice.DumpKVVMIMigrationConfiguration(kvvmi), "conf", internalservice.DumpMigrationConfiguration(conf))

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
