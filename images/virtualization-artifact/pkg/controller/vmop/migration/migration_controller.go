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

package migration

import (
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/watcher"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	controllerName = "vmop-migration-controller"
)

func NewController(client client.Client, mgr manager.Manager, featureGate featuregate.FeatureGate) *Controller {
	recorder := eventrecord.NewEventRecorderLogger(mgr, controllerName)
	baseSvc := genericservice.NewBaseVMOPService(client, recorder)
	migration := service.NewMigrationService(client, featureGate)
	return &Controller{
		watchers: []reconciler.Watcher{
			watcher.NewVMOPWatcher(),
			watcher.NewMigrationWatcher(),
			watcher.NewVMWatcher(),
		},
		handlers: []reconciler.Handler[*v1alpha2.VirtualMachineOperation]{
			handler.NewDeletionHandler(migration),
			handler.NewLifecycleHandler(client, migration, baseSvc, recorder),
		},
	}
}

type Controller struct {
	watchers []reconciler.Watcher
	handlers []reconciler.Handler[*v1alpha2.VirtualMachineOperation]
}

func (c *Controller) Name() string {
	return controllerName
}

func (c *Controller) Watchers() []reconciler.Watcher {
	return c.watchers
}

func (c *Controller) Handlers() []reconciler.Handler[*v1alpha2.VirtualMachineOperation] {
	return c.handlers
}

func (c *Controller) ShouldReconcile(vmop *v1alpha2.VirtualMachineOperation) bool {
	return watcher.Match(vmop)
}
