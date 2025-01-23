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
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachinesWatcher struct {
	log *slog.Logger
}

func NewVirtualMachinesWatcher() *VirtualMachinesWatcher {
	return &VirtualMachinesWatcher{
		log: slog.Default().With("watcher", strings.ToLower(virtv2.VirtualMachineClassKind)),
	}
}

func (w *VirtualMachinesWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			vm, ok := obj.(*virtv2.VirtualMachine)
			if !ok {
				w.log.Error(fmt.Sprintf("expected a new VirtualMachine but got a %T", obj))
				return nil
			}

			c := mgr.GetClient()
			vmc := &virtv2.VirtualMachineClass{}
			err := c.Get(ctx, types.NamespacedName{
				Name: vm.Spec.VirtualMachineClassName,
			}, vmc)
			if err != nil {
				w.log.Error(
					"error retrieving virtual machines during the search for virtual machine class belonging changed virtual machine",
					logger.SlogErr(err),
				)
				return nil
			}

			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: vmc.Name,
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				return false
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				return true
			},
		},
	)
}
