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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachinesWatcher struct{}

func NewVirtualMachinesWatcher() *VirtualMachinesWatcher {
	return &VirtualMachinesWatcher{}
}

func (w *VirtualMachinesWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	mgrClient := mgr.GetClient()
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vm *v1alpha2.VirtualMachine) []reconcile.Request {
				vmClassName := vm.Spec.VirtualMachineClassName
				vmc, err := object.FetchObject(ctx, types.NamespacedName{
					Name: vmClassName,
				}, mgrClient, &v1alpha2.VirtualMachineClass{})

				if vmc == nil {
					return nil
				}

				if err != nil {
					slog.Default().Error("failed to fetch virtual machine class", slog.String("vmClassName", vmClassName), logger.SlogErr(err))
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
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachine]) bool {
					return false
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VMs: %w", err)
	}
	return nil
}
