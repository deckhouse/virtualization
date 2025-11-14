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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMWatcher() *VMWatcher {
	return &VMWatcher{}
}

type VMWatcher struct{}

func (w VMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	mgrClient := mgr.GetClient()
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineSnapshot{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot) []reconcile.Request {
				vmsops := &v1alpha2.VirtualMachineSnapshotOperationList{}
				if err := mgrClient.List(ctx, vmsops, client.InNamespace(vms.GetNamespace())); err != nil {
					return nil
				}
				var requests []reconcile.Request
				for _, vmsop := range vmsops.Items {
					if !Match(&vmsop) {
						continue
					}

					if vmsop.Spec.VirtualMachineSnapshotName == vms.GetName() && vmsop.Status.Phase == v1alpha2.VMSOPPhaseInProgress {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: vmsop.GetNamespace(),
								Name:      vmsop.GetName(),
							},
						})
						break
					}
				}
				return requests
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineSnapshot]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineSnapshot]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase || !equality.Semantic.DeepEqual(e.ObjectOld.Status.Conditions, e.ObjectNew.Status.Conditions)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}
