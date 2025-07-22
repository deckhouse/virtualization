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
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineSnapshotWatcher struct {
	client client.Client
}

func NewVirtualMachineSnapshotWatcher(client client.Client) *VirtualMachineSnapshotWatcher {
	return &VirtualMachineSnapshotWatcher{
		client: client,
	}
}

func (w VirtualMachineSnapshotWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineSnapshot{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w VirtualMachineSnapshotWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var vmRestores virtv2.VirtualMachineRestoreList
	err := w.client.List(ctx, &vmRestores, &client.ListOptions{
		Namespace:     obj.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMRestoreByVMSnapshot, obj.GetName()),
	})
	if err != nil {
		log.Error(fmt.Sprintf("failed to list virtual machine restores: %s", err))
		return
	}

	for _, vmRestore := range vmRestores.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vmRestore.Name,
				Namespace: vmRestore.Namespace,
			},
		})
	}

	return
}

func (w VirtualMachineSnapshotWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldVMSnapshot, ok := e.ObjectOld.(*virtv2.VirtualMachineSnapshot)
	if !ok {
		log.Error(fmt.Sprintf("expected an old VirtualMachineSnapshot but got a %T", e.ObjectOld))
		return false
	}

	newVMSnapshot, ok := e.ObjectNew.(*virtv2.VirtualMachineSnapshot)
	if !ok {
		log.Error(fmt.Sprintf("expected a new VirtualMachineSnapshot but got a %T", e.ObjectNew))
		return false
	}

	return oldVMSnapshot.Status.Phase != newVMSnapshot.Status.Phase
}
