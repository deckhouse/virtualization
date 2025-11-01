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
	"fmt"
	"time"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal"
	vmservice "github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
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
	ns string,
	queue <-chan reconcile.Request,
) error {
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName+"-"+ns)
	client := mgr.GetClient()
	blockDeviceService := service.NewBlockDeviceService(client)

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
		internal.NewSyncPowerStateHandler(client, recorder),
		internal.NewSyncMetadataHandler(client),
		internal.NewLifeCycleHandler(client, recorder),
		internal.NewMigratingHandler(migrateVolumesService),
		internal.NewFirmwareHandler(firmwareImage),
		internal.NewEvictHandler(),
		internal.NewStatisticHandler(client),
	}

	r := NewReconciler(client, handlers...)

	options := controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
			5*time.Second,
			5*time.Second,
		),
	}
	options.DefaultFromConfig(mgr.GetControllerOptions())
	c, err := controller.NewTypedUnmanaged(ControllerName+"-"+ns, options)
	if err != nil {
		return fmt.Errorf("new typed unmanaged controller failed: %w", err)
	}

	if err = r.SetupController(ctx, c, queue); err != nil {
		return err
	}

	go func() {
		err = c.Start(ctx)
		if err != nil {
			log.Error(fmt.Errorf("error starting controller %q: %w", ControllerName+"-"+ns, err).Error())
		}
	}()

	log.Info(fmt.Sprintf("the NamespacedVirtualMachine controller has been started for %q", ns))

	return nil
}
