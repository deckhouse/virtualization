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
	"log/slog"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	vmmetrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/virtualmachine"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	controllerName = "vm-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *slog.Logger,
	dvcrSettings *dvcr.Settings,
) (controller.Controller, error) {
	if log == nil {
		log = slog.Default()
	}
	logger := log.With("controller", controllerName)
	recorder := mgr.GetEventRecorderFor(controllerName)
	mgrCache := mgr.GetCache()
	client := mgr.GetClient()
	handlers := []Handler{
		internal.NewDeletionHandler(client, logger),
		internal.NewClassHandler(client, recorder, logger),
		internal.NewIPAMHandler(ipam.New(), client, recorder, logger),
		internal.NewBlockDeviceHandler(client, recorder, logger),
		internal.NewProvisioningHandler(client),
		internal.NewAgentHandler(),
		internal.NewPodHandler(client),
		internal.NewSyncKvvmHandler(dvcrSettings, client, recorder, logger),
		internal.NewSyncMetadataHandler(client),
		internal.NewLifeCycleHandler(client, recorder, logger),
		internal.NewStatisticHandler(client),
	}
	r := NewReconciler(client, logger, handlers...)

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r, RecoverPanic: ptr.To(true)})
	if err != nil {
		return nil, err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachine{}).
		WithValidator(NewValidator(ipam.New(), mgr.GetClient(), logger)).
		Complete(); err != nil {
		return nil, err
	}

	vmmetrics.SetupCollector(&vmLister{vmCache: mgrCache}, metrics.Registry)

	log.Info("Initialized VirtualMachine controller")
	return c, nil
}

type vmLister struct {
	vmCache cache.Cache
}

func (l vmLister) List() ([]v1alpha2.VirtualMachine, error) {
	vmList := v1alpha2.VirtualMachineList{}
	err := l.vmCache.List(context.Background(), &vmList)
	if err != nil {
		return nil, err
	}
	return vmList.Items, nil
}
