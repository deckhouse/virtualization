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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewVirtualMachineWatcher(client client.Client) *VirtualMachineWatcher {
	return &VirtualMachineWatcher{
		logger: log.Default().With("watcher", "vm"),
		client: client,
	}
}

func (w VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachine]) bool {
					return w.hasVirtualImageRef(e.Object)
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachine]) bool {
					return w.hasVirtualImageRef(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					return w.hasVirtualImageRef(e.ObjectOld) || w.hasVirtualImageRef(e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}

func (w VirtualMachineWatcher) enqueueRequests(ctx context.Context, vm *v1alpha2.VirtualMachine) (requests []reconcile.Request) {
	for _, ref := range vm.Status.BlockDeviceRefs {
		if ref.Kind != v1alpha2.ImageDevice {
			continue
		}

		vi, err := object.FetchObject(ctx, types.NamespacedName{
			Namespace: vm.Namespace,
			Name:      ref.Name,
		}, w.client, &v1alpha2.VirtualImage{})
		if err != nil {
			w.logger.Error("Failed to fetch vi to reconcile", logger.SlogErr(err))
			continue
		}

		if vi == nil {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vi.Name,
				Namespace: vi.Namespace,
			},
		})
	}

	return
}

func (w VirtualMachineWatcher) hasVirtualImageRef(vm *v1alpha2.VirtualMachine) bool {
	for _, ref := range vm.Spec.BlockDeviceRefs {
		if ref.Kind == v1alpha2.ImageDevice {
			return true
		}
	}

	for _, ref := range vm.Status.BlockDeviceRefs {
		if ref.Kind == v1alpha2.ImageDevice {
			return true
		}
	}

	return false
}
