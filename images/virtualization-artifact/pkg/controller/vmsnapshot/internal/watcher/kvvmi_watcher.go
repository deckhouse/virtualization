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
	"log/slog"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type KVVMIWatcher struct {
	client client.Client
}

func NewKVVMIWatcher(client client.Client) *KVVMIWatcher {
	return &KVVMIWatcher{
		client: client,
	}
}

func (w KVVMIWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv1.VirtualMachineInstance]{
				UpdateFunc: w.filterUpdateEvents,
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on KVVMI: %w", err)
	}
	return nil
}

func (w KVVMIWatcher) enqueueRequests(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) (requests []reconcile.Request) {
	var vmSnapshots v1alpha2.VirtualMachineSnapshotList
	err := w.client.List(ctx, &vmSnapshots, &client.ListOptions{
		Namespace:     kvvmi.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMSnapshotByVM, kvvmi.GetName()),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list virtual machine snapshots: %s", err))
		return
	}

	for _, vmSnapshot := range vmSnapshots.Items {
		if vmSnapshot.Spec.VirtualMachineName == kvvmi.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      vmSnapshot.Name,
					Namespace: vmSnapshot.Namespace,
				},
			})
		}
	}

	return
}

func (w KVVMIWatcher) filterUpdateEvents(e event.TypedUpdateEvent[*virtv1.VirtualMachineInstance]) bool {
	oldFSFrozen := e.ObjectOld.Status.FSFreezeStatus
	newFSFrozen := e.ObjectNew.Status.FSFreezeStatus

	if oldFSFrozen != newFSFrozen {
		return true
	}

	oldRequest, oldOk := e.ObjectOld.Annotations[annotations.AnnVMFilesystemRequest]
	newRequest, newOk := e.ObjectNew.Annotations[annotations.AnnVMFilesystemRequest]

	if oldOk && newOk {
		return oldRequest != newRequest
	}

	return oldOk != newOk
}
