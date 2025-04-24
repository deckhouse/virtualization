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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMOPWatcher struct{}

func NewVMOPWatcher() *VMOPWatcher {
	return &VMOPWatcher{}
}

func (w *VMOPWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineOperation{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
			vmop, ok := object.(*v1alpha2.VirtualMachineOperation)
			if !ok {
				return nil
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      vmop.Spec.VirtualMachine,
						Namespace: vmop.Namespace,
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVMOP, oldOK := e.ObjectOld.(*v1alpha2.VirtualMachineOperation)
				newVMOP, newOK := e.ObjectNew.(*v1alpha2.VirtualMachineOperation)
				if !oldOK || !newOK {
					return false
				}
				_, isEvacuation := newVMOP.GetAnnotations()[annotations.AnnVMOPEvacuation]
				return isEvacuation && oldVMOP.Status.Phase != newVMOP.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineOperation: %w", err)
	}
	return nil
}
