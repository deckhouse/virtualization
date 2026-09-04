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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMWatcher() *VMWatcher {
	return &VMWatcher{}
}

type VMWatcher struct{}

// Watch follows the machines that hold the node: the request is born and dies with them, so the
// outcome reported by a machine is what moves the dialogue on.
func (w *VMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(requestByNode),
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					return commonvm.HoldsNodeUnderMaintenance(e.ObjectOld) != commonvm.HoldsNodeUnderMaintenance(e.ObjectNew) ||
						e.ObjectOld.Status.Node != e.ObjectNew.Status.Node
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}

	return nil
}

func requestByNode(_ context.Context, vm *v1alpha2.VirtualMachine) []reconcile.Request {
	if vm == nil || vm.Status.Node == "" {
		return nil
	}

	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: vm.Status.Node}}}
}
