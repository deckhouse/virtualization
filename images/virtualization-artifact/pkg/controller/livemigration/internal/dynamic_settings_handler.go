package internal

import (
	"context"

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/livemigration/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
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

	// Get vm
	// Get vmclass
	// Get vmop
	// Merge live migration policies from 3 sources.
	// Add global live migration settings
	// Patch vmi status with migrationConfiguration

	if !h.shouldUpdateMigrationConfiguration(kvvmi) {
		return reconcile.Result{}, nil
	}

	conf := service.NewMigrationConfiguration(h.moduleSettings)

	kvvmi.Status.MigrationState.MigrationConfiguration = conf

	log.Info("Patch KVVMI with migration settings", "migration.configuration", service.DumpMigrationConfiguration(conf))

	return reconcile.Result{}, nil
}

func (h *DynamicSettingsHandler) Name() string {
	return dynamicSettingsHandlerName
}

// shouldUpdateMigrationConfiguration indicates if live migration controller should inject
// migration configuration into KVVMI status:
// 1. status.migrationState is created by the virt-controller.
// 2. status.migrationState.migrationConfiguration was not set by previous reconciles.
// 3. migration is not in a Completed state.
func (h *DynamicSettingsHandler) shouldUpdateMigrationConfiguration(kvvmi *virtv1.VirtualMachineInstance) bool {
	return kvvmi.Status.MigrationState != nil &&
		kvvmi.Status.MigrationState.MigrationConfiguration == nil &&
		!kvvmi.Status.MigrationState.Completed
}
