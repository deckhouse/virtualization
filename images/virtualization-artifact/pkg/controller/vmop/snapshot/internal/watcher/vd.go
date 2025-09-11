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

func NewVDWatcher() *VirtualDiskWatcher {
	return &VirtualDiskWatcher{}
}

type VirtualDiskWatcher struct{}

func (w VirtualDiskWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	mgrClient := mgr.GetClient()
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualDisk{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vd *v1alpha2.VirtualDisk) []reconcile.Request {
				// Use when we deleting VD before creating new one. Order is important because VD may have restore annotation from previous restore.
				restoreUID, hasRestoreAnnotation := vd.Annotations[annotations.AnnVMOPRestoreDeleted]
				if !hasRestoreAnnotation {
					restoreUID, hasRestoreAnnotation = vd.Annotations[annotations.AnnVMOPRestore]
				}

				if !hasRestoreAnnotation {
					return nil
				}

				// Find VMOPs that match this restore operation
				vmops := &v1alpha2.VirtualMachineOperationList{}
				if err := mgrClient.List(ctx, vmops, client.InNamespace(vd.GetNamespace())); err != nil {
					return nil
				}

				var requests []reconcile.Request
				for _, vmop := range vmops.Items {
					if !Match(&vmop) {
						continue
					}

					// Check if this VMOP matches the restore UID and is in progress
					if string(vmop.UID) == restoreUID && vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: vmop.GetNamespace(),
								Name:      vmop.GetName(),
							},
						})
					}
				}
				return requests
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualDisk]{
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualDisk]) bool {
					// Always trigger on delete events - the handler will filter for relevant VMOPs
					// since deleted VDs might not have restore annotations but could still be blocking restores
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualDisk]) bool {
					// Trigger reconciliation when VirtualDisk phase changes during restore
					_, hasRestoreAnnotation := e.ObjectNew.Annotations[annotations.AnnVMOPRestore]
					return hasRestoreAnnotation && e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
	}
	return nil
}
