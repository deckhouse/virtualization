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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMWatcher() *VMWatcher {
	return &VMWatcher{}
}

type VMWatcher struct{}

func (w VMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, vm client.Object) []reconcile.Request {
			c := mgr.GetClient()
			vmops := &v1alpha2.VirtualMachineOperationList{}
			if err := c.List(ctx, vmops, client.InNamespace(vm.GetNamespace())); err != nil {
				return nil
			}
			var requests []reconcile.Request
			for _, vmop := range vmops.Items {
				if vmop.Spec.VirtualMachine == vm.GetName() && vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vmop.GetNamespace(),
							Name:      vmop.GetName(),
						},
					})
					break
				}
			}
			return requests
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVM := e.ObjectOld.(*v1alpha2.VirtualMachine)
				newVM := e.ObjectNew.(*v1alpha2.VirtualMachine)
				return oldVM.Status.Phase != newVM.Status.Phase || newVM.Status.MigrationState != nil
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}
