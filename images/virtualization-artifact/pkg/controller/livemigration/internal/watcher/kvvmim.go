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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func NewKVVMIMWatcher() *KVVMIWatcher {
	return &KVVMIWatcher{}
}

type KVVMIMWatcher struct{}

func (w *KVVMIMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	// Subscribe to VirtualMachineInstanceMigration status changes.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstanceMigration{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, kvvmim *virtv1.VirtualMachineInstanceMigration) []reconcile.Request {
				vmiName := kvvmim.Spec.VMIName
				if vmiName == "" {
					return nil
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      vmiName,
							Namespace: kvvmim.GetNamespace(),
						},
					},
				}
			}),
			predicate.TypedFuncs[*virtv1.VirtualMachineInstanceMigration]{
				DeleteFunc: func(e event.TypedDeleteEvent[*virtv1.VirtualMachineInstanceMigration]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv1.VirtualMachineInstanceMigration]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineInstanceMigration: %w", err)
	}

	return nil
}
