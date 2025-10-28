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

package snapshot

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/snapshot/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/snapshot/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	controllerName = "vmsop-snapshot-controller"
)

func NewController(client client.Client, mgr manager.Manager) *Controller {
	recorder := eventrecord.NewEventRecorderLogger(mgr, controllerName)
	baseSvc := genericservice.NewBaseVMSOPService(client, recorder)
	svcOpCreator := handler.NewSvcOpCreator(client, recorder)

	return &Controller{
		watchers: []reconciler.Watcher{
			watcher.NewVMWatcher(),
			watcher.NewVMSOPWatcher(),
			watcher.NewVDWatcher(),
			watcher.NewVMBDAWatcher(),
			watcher.NewVMSnapshotWatcher(),
		},
		handlers: []reconciler.Handler[*v1alpha2.VirtualMachineSnapshotOperation]{
			handler.NewLifecycleHandler(svcOpCreator, baseSvc, recorder),
			handler.NewDeletionHandler(client),
		},
	}
}

type Controller struct {
	watchers []reconciler.Watcher
	handlers []reconciler.Handler[*v1alpha2.VirtualMachineSnapshotOperation]
}

func (c *Controller) Name() string {
	return controllerName
}

func (c *Controller) Watchers() []reconciler.Watcher {
	return c.watchers
}

func (c *Controller) Handlers() []reconciler.Handler[*v1alpha2.VirtualMachineSnapshotOperation] {
	return c.handlers
}

func (c *Controller) ShouldReconcile(vmsop *v1alpha2.VirtualMachineSnapshotOperation) bool {
	return watcher.Match(vmsop)
}
