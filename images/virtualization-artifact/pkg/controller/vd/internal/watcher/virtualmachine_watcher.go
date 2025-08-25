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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineWatcher struct{}

func NewVirtualMachineWatcher() *VirtualMachineWatcher {
	return &VirtualMachineWatcher{}
}

func (w *VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueDisksAttachedToVM),
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachine]) bool {
					return w.vmHasAttachedDisks(e.Object)
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachine]) bool {
					return w.vmHasAttachedDisks(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					return w.vmHasAttachedDisks(e.ObjectOld) || w.vmHasAttachedDisks(e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VMs: %w", err)
	}
	return nil
}

func (w *VirtualMachineWatcher) enqueueDisksAttachedToVM(_ context.Context, vm *v1alpha2.VirtualMachine) []reconcile.Request {
	var requests []reconcile.Request

	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind != v1alpha2.DiskDevice {
			continue
		}

		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      bdr.Name,
			Namespace: vm.Namespace,
		}})
	}

	return requests
}

func (w *VirtualMachineWatcher) vmHasAttachedDisks(vm *v1alpha2.VirtualMachine) bool {
	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == v1alpha2.DiskDevice {
			return true
		}
	}

	return false
}
