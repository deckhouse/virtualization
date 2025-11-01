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

func NewVMIPWatcher() *VMIPWatcher {
	return &VMIPWatcher{}
}

type VMIPWatcher struct{}

func (w *VMIPWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachineIPAddress{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) []reconcile.Request {
				name := vmip.Status.VirtualMachine
				if name == "" {
					return nil
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      name,
							Namespace: vmip.GetNamespace(),
						},
					},
				}
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineIPAddress]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineIPAddress]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						e.ObjectOld.Status.VirtualMachine != e.ObjectNew.Status.VirtualMachine
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineIpAddress: %w", err)
	}
	return nil
}
