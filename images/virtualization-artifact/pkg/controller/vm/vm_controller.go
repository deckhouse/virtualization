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

package vm

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal"
	vmservice "github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	vmmetrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/virtualmachine"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "vm-controller"
)

func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	dvcrSettings *dvcr.Settings,
	firmwareImage string,
	virtClient service.VirtClient,
	controllerNamespace string,
) error {
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)
	mgrCache := mgr.GetCache()
	client := mgr.GetClient()
	blockDeviceService := service.NewBlockDeviceService(client)
	vmClassService := service.NewVirtualMachineClassService(client)
	attachmentService := service.NewAttachmentService(client, virtClient, controllerNamespace)

	migrateVolumesService := vmservice.NewMigrationVolumesService(client, internal.MakeKVVMFromVMSpec, 10*time.Second)

	handlers := []Handler{
		internal.NewMaintenanceHandler(client),
		internal.NewDeletionHandler(client),
		internal.NewClassHandler(client, recorder),
		internal.NewIPAMHandler(netmanager.NewIPAM(), client, recorder),
		internal.NewMACHandler(netmanager.NewMACManager(), client, recorder),
		internal.NewBlockDeviceHandler(client, blockDeviceService),
		internal.NewProvisioningHandler(client),
		internal.NewAgentHandler(),
		internal.NewFilesystemHandler(),
		internal.NewSnapshottingHandler(client),
		internal.NewPodHandler(client),
		internal.NewSizePolicyHandler(),
		internal.NewNetworkInterfaceHandler(featuregates.Default()),
		internal.NewSyncKvvmHandler(dvcrSettings, client, recorder, migrateVolumesService),
		internal.NewHotplugHandler(attachmentService),
		internal.NewSyncPowerStateHandler(client, recorder),
		internal.NewSyncMetadataHandler(client),
		internal.NewLifeCycleHandler(client, recorder),
		internal.NewMigratingHandler(migrateVolumesService),
		internal.NewFirmwareHandler(firmwareImage),
		internal.NewEvictHandler(),
		internal.NewStatisticHandler(client),
	}
	r := NewReconciler(client, handlers...)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachine{}).
		WithValidator(NewValidator(client, blockDeviceService, featuregates.Default(), log)).
		WithDefaulter(NewDefaulter(client, vmClassService, log)).
		Complete(); err != nil {
		return err
	}

	vmmetrics.SetupCollector(mgrCache, metrics.Registry, log)

	log.Info("Initialized VirtualMachine controller")
	return nil
}
