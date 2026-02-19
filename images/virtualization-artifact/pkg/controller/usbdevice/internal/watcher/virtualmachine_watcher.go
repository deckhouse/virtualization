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

package watcher

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
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

func NewVirtualMachineWatcher() *VirtualMachineWatcher {
	return &VirtualMachineWatcher{}
}

type VirtualMachineWatcher struct{}

func (w *VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vm *v1alpha2.VirtualMachine) []reconcile.Request {
				var result []reconcile.Request
				for _, ref := range vm.Spec.USBDevices {
					result = append(result, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vm.Namespace,
							Name:      ref.Name,
						},
					})
				}
				return result
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachine]) bool {
					return true
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachine]) bool {
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					return shouldProcessVirtualMachineUpdate(e.ObjectOld, e.ObjectNew)
				},
			},
		),
	)
}

func shouldProcessVirtualMachineUpdate(oldObj, newObj *v1alpha2.VirtualMachine) bool {
	if oldObj == nil || newObj == nil {
		return false
	}

	return !equality.Semantic.DeepEqual(oldObj.Spec.USBDevices, newObj.Spec.USBDevices) ||
		!equality.Semantic.DeepEqual(oldObj.Status.USBDevices, newObj.Status.USBDevices) ||
		!equality.Semantic.DeepEqual(oldObj.GetDeletionTimestamp(), newObj.GetDeletionTimestamp())
}
