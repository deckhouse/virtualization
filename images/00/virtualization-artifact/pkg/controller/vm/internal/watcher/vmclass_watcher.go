/*
Copyright 2024 Flant JSC

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineClassWatcher struct{}

func NewVirtualMachineClassWatcher() *VirtualMachineClassWatcher {
	return &VirtualMachineClassWatcher{}
}

func (w VirtualMachineClassWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
			c := mgr.GetClient()
			vms := &virtv2.VirtualMachineList{}
			err := c.List(ctx, vms, client.MatchingFields{
				indexer.IndexFieldVMByClass: object.GetName(),
			})
			if err != nil {
				log := logger.FromContext(ctx)
				log.Error(
					"error retrieving virtual machines during the search for virtual machines belonging changed class",
					logger.SlogErr(err),
				)
				return nil
			}

			requests := make([]reconcile.Request, len(vms.Items))
			for i, vm := range vms.Items {
				requests[i] = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      vm.Name,
						Namespace: vm.Namespace,
					},
				}
			}
			return requests
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return false },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVMC, oldOk := e.ObjectOld.(*virtv2.VirtualMachineClass)
				newVMC, newOk := e.ObjectNew.(*virtv2.VirtualMachineClass)
				if !oldOk || !newOk {
					return false
				}
				return !equality.Semantic.DeepEqual(oldVMC.Spec.SizingPolicies, newVMC.Spec.SizingPolicies) ||
					!equality.Semantic.DeepEqual(oldVMC.Spec.Tolerations, oldVMC.Spec.Tolerations) ||
					!equality.Semantic.DeepEqual(oldVMC.Spec.NodeSelector, newVMC.Spec.NodeSelector)
			},
		},
	)
}
