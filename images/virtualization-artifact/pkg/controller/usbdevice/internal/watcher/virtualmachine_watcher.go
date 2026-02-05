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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
				seen := make(map[string]struct{})
				for _, ref := range vm.Spec.USBDevices {
					if _, exists := seen[ref.Name]; exists {
						continue
					}
					seen[ref.Name] = struct{}{}
					result = append(result, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vm.Namespace,
							Name:      ref.Name,
						},
					})
				}
				return result
			}),
		),
	)
}
