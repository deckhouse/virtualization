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

package watcher

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMWatcher struct {
	log *slog.Logger
}

func NewVMWatcher(log *slog.Logger) *VMWatcher {
	return &VMWatcher{
		log: log,
	}
}

func (w *VMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	c := mgr.GetClient()
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vm *v1alpha2.VirtualMachine) []reconcile.Request {
				var result []reconcile.Request

				for _, bd := range vm.Spec.BlockDeviceRefs {
					if bd.Kind != v1alpha2.DiskDevice {
						continue
					}

					vd := &v1alpha2.VirtualDisk{}
					err := c.Get(ctx, client.ObjectKey{Namespace: vm.Namespace, Name: bd.Name}, vd)
					if err != nil {
						w.log.Error("failed to get VirtualDisk", logger.SlogErr(err))
						return nil
					}

					if commonvd.StorageClassChanged(vd) {
						result = append(result, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(vd)})
					}
				}

				return result
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachine]) bool {
					return false
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					if !(e.ObjectOld.Status.Phase != v1alpha2.MachineRunning && e.ObjectNew.Status.Phase == v1alpha2.MachineRunning) {
						return false
					}
					for _, bd := range e.ObjectNew.Spec.BlockDeviceRefs {
						if bd.Kind == v1alpha2.DiskDevice {
							return true
						}
					}
					return false
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachine]) bool {
					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
	}
	return nil
}
