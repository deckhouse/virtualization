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

package nodeusbdevice

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodeusbdevice/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, s state.NodeUSBDeviceState) (reconcile.Result, error)
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
			&v1alpha2.NodeUSBDevice{},
			&handler.TypedEnqueueRequestForObject[*v1alpha2.NodeUSBDevice]{},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on NodeUSBDevice: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewResourceSliceWatcher(),
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

	nodeUSBDevice := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := nodeUSBDevice.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	s := state.New(r.client, nodeUSBDevice)

	// If NodeUSBDevice doesn't exist, only run DiscoveryHandler to discover new devices
	if nodeUSBDevice.IsEmpty() {
		log.Info("Reconcile observe an absent NodeUSBDevice: running discovery to find new devices")

		// Find DiscoveryHandler and run it
		for _, handler := range r.handlers {
			if handler.Name() == "DiscoveryHandler" {
				result, err := handler.Handle(ctx, s)
				if err != nil {
					log.Error("DiscoveryHandler failed", slog.Attr{Key: "error", Value: slog.StringValue(err.Error())})
				}
				return result, err
			}
		}

		// If DiscoveryHandler not found, return
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		changed := nodeUSBDevice.Changed()
		changed.Status.ObservedGeneration = changed.Generation

		// If changed has empty Attributes (e.g. due to cache delay after create), fetch latest from server
		// so we don't overwrite the status set in createNodeUSBDevice with empty Attributes.
		if changed.Status.Attributes.Name == "" {
			latest := &v1alpha2.NodeUSBDevice{}
			if err := r.client.Get(ctx, nodeUSBDevice.Name(), latest); err == nil && latest.Status.Attributes.Name != "" {
				changed.Status.Attributes = latest.Status.Attributes
				changed.Status.NodeName = latest.Status.NodeName
			}
		}

		return nodeUSBDevice.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *v1alpha2.NodeUSBDevice {
	return &v1alpha2.NodeUSBDevice{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.NodeUSBDevice) v1alpha2.NodeUSBDeviceStatus {
	return obj.Status
}
