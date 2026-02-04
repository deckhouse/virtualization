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

package usbdevice

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, s state.USBDeviceState) (reconcile.Result, error)
	Name() string
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

type Reconciler struct {
	client   client.Client
	handlers []Handler
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(),
			&v1alpha2.USBDevice{},
			&handler.TypedEnqueueRequestForObject[*v1alpha2.USBDevice]{},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on USBDevice: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewNodeUSBDeviceWatcher(),
		watcher.NewResourceClaimTemplateWatcher(),
		watcher.NewVirtualMachineWatcher(),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	usbDevice := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := usbDevice.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if usbDevice.IsEmpty() {
		log.Info("Reconcile observe an absent USBDevice: it may be deleted")
		return reconcile.Result{}, nil
	}

	s := state.New(r.client, usbDevice)

	rec := reconciler.NewBaseReconciler(r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		usbDevice.Changed().Status.ObservedGeneration = usbDevice.Changed().Generation

		return usbDevice.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *v1alpha2.USBDevice {
	return &v1alpha2.USBDevice{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus {
	return obj.Status
}
