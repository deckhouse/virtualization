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

package supervm

import (
	"context"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supervm/internal"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	vmmetrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/virtualmachine"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "super-vm-controller"
)

func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	dvcrSettings *dvcr.Settings,
	firmwareImage string,
	logFactory logger.Factory,
	log *log.Logger,
) error {
	cache := mgr.GetCache()
	client := mgr.GetClient()
	blockDeviceService := service.NewBlockDeviceService(client)
	vmClassService := service.NewVirtualMachineClassService(client)

	r := NewReconciler(client, internal.NewSemaphoreHandler(
		mgr,
		dvcrSettings,
		firmwareImage,
		logFactory,
	))
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
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

	vmmetrics.SetupCollector(cache, metrics.Registry, log)

	log.Info("Initialized SuperVirtualMachine controller")
	return nil
}
