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

// MigrationWatcher enqueues the virtual machine a migration moves when the migration appears, finishes
// or goes away.
type MigrationWatcher struct{}

func NewMigrationWatcher() *MigrationWatcher {
	return &MigrationWatcher{}
}

func (w *MigrationWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstanceMigration{},
			handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, vmim *virtv1.VirtualMachineInstanceMigration) []reconcile.Request {
				if vmim.Spec.VMIName == "" {
					return nil
				}

				return []reconcile.Request{{NamespacedName: types.NamespacedName{
					Namespace: vmim.Namespace,
					Name:      vmim.Spec.VMIName,
				}}}
			}),
			predicate.TypedFuncs[*virtv1.VirtualMachineInstanceMigration]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv1.VirtualMachineInstanceMigration]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on internal VirtualMachineInstanceMigration: %w", err)
	}

	return nil
}
