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

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewVMOPWatcher() *VMOPWatcher {
	return &VMOPWatcher{}
}

type VMOPWatcher struct{}

func (w *VMOPWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachineOperation{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      vmop.Spec.VirtualMachine,
							Namespace: vmop.GetNamespace(),
						},
					},
				}
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineOperation]{
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachineOperation]) bool {
					return commonvmop.IsMigration(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineOperation]) bool {
					oldCompleted, _ := conditions.GetCondition(vmopcondition.TypeCompleted, e.ObjectOld.Status.Conditions)
					newCompleted, _ := conditions.GetCondition(vmopcondition.TypeCompleted, e.ObjectNew.Status.Conditions)

					return oldCompleted.Reason != newCompleted.Reason
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineOperation: %w", err)
	}
	return nil
}
