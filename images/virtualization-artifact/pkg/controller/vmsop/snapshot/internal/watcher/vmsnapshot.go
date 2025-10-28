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

func NewVMSnapshotWatcher() *VirtualMachineSnapshotWatcher {
	return &VirtualMachineSnapshotWatcher{}
}

type VirtualMachineSnapshotWatcher struct{}

func (w VirtualMachineSnapshotWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	mgrClient := mgr.GetClient()
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineSnapshot{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vd *v1alpha2.VirtualMachineSnapshot) []reconcile.Request {
				vmsopUID, hasVMSOPAnnotation := vd.Annotations[annotations.AnnVMOPUID]
				if !hasVMSOPAnnotation {
					return nil
				}

				// Find VMSOPs that match this restore operation
				vmsops := &v1alpha2.VirtualMachineSnapshotOperationList{}
				if err := mgrClient.List(ctx, vmsops, client.InNamespace(vd.GetNamespace())); err != nil {
					return nil
				}

				var requests []reconcile.Request
				for _, vmsop := range vmsops.Items {
					if !Match(&vmsop) {
						continue
					}

					// Check if this VMSOP matches the restore UID and is in progress
					if string(vmsop.UID) == vmsopUID {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: vmsop.GetNamespace(),
								Name:      vmsop.GetName(),
							},
						})
					}
				}
				return requests
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineSnapshot]{
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachineSnapshot]) bool {
					// Always trigger on delete events - the handler will filter for relevant VMSOPs
					// since deleted VMSnapshots might not have annotations but could still be blocking restores
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineSnapshot]) bool {
					// Trigger reconciliation when VirtualMachineSnapshot phase changes during restore
					_, hasVMSOPUIDAnnotation := e.ObjectNew.Annotations[annotations.AnnVMOPUID]
					return hasVMSOPUIDAnnotation && e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineSnapshot: %w", err)
	}
	return nil
}
