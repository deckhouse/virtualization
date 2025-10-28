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

func NewVMBDAWatcher() *VMBlockDeviceAttachmentWatcher {
	return &VMBlockDeviceAttachmentWatcher{}
}

type VMBlockDeviceAttachmentWatcher struct{}

func (w VMBlockDeviceAttachmentWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	mgrClient := mgr.GetClient()
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineBlockDeviceAttachment{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) []reconcile.Request {
				// Use when we deleting VMBDA before creating new one. Order is important because VMBDA may have restore annotation from previous restore.
				restoreUID, hasRestoreAnnotation := vmbda.Annotations[annotations.AnnVMOPRestoreDeleted]
				if !hasRestoreAnnotation {
					restoreUID, hasRestoreAnnotation = vmbda.Annotations[annotations.AnnVMOPRestore]
				}

				if !hasRestoreAnnotation {
					return nil
				}

				// Find VMSOPs that match this restore operation
				vmsops := &v1alpha2.VirtualMachineSnapshotOperationList{}
				if err := mgrClient.List(ctx, vmsops, client.InNamespace(vmbda.GetNamespace())); err != nil {
					return nil
				}

				var requests []reconcile.Request
				for _, vmsop := range vmsops.Items {
					if !Match(&vmsop) {
						continue
					}

					// Check if this VMSOP matches the restore UID and is in progress
					if string(vmsop.UID) == restoreUID && vmsop.Status.Phase == v1alpha2.VMSOPPhaseInProgress {
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
			predicate.TypedFuncs[*v1alpha2.VirtualMachineBlockDeviceAttachment]{
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachineBlockDeviceAttachment]) bool {
					// Always trigger on delete events - the handler will filter for relevant VMSOPs
					// since deleted VMBDAs might not have restore annotations but could still be blocking restores
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineBlockDeviceAttachment]) bool {
					// Trigger reconciliation when VMBDA phase changes during restore
					_, hasRestoreAnnotation := e.ObjectNew.Annotations[annotations.AnnVMOPRestore]
					return hasRestoreAnnotation && e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineBlockDeviceAttachment: %w", err)
	}
	return nil
}
