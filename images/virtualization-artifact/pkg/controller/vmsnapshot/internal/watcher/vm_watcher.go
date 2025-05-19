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
	"log/slog"

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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type VirtualMachineWatcher struct {
	client client.Client
}

func NewVirtualMachineWatcher(client client.Client) *VirtualMachineWatcher {
	return &VirtualMachineWatcher{
		client: client,
	}
}

func (w VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w VirtualMachineWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var vmSnapshots virtv2.VirtualMachineSnapshotList
	err := w.client.List(ctx, &vmSnapshots, &client.ListOptions{
		Namespace:     obj.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMSnapshotByVM, obj.GetName()),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list virtual machine snapshots: %s", err))
		return
	}

	for _, vmSnapshot := range vmSnapshots.Items {
		if vmSnapshot.Spec.VirtualMachineName == obj.GetName() {
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

func (w VirtualMachineWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldVM, ok := e.ObjectOld.(*virtv2.VirtualMachine)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected an old VirtualMachine but got a %T", e.ObjectOld))
		return false
	}

	newVM, ok := e.ObjectNew.(*virtv2.VirtualMachine)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected a new VirtualMachine but got a %T", e.ObjectNew))
		return false
	}

	oldAgentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, oldVM.Status.Conditions)
	newAgentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, newVM.Status.Conditions)

	if oldAgentReady != newAgentReady {
		return true
	}

	oldFSFrozen, _ := conditions.GetCondition(vmcondition.TypeFilesystemFrozen, oldVM.Status.Conditions)
	newFSFrozen, _ := conditions.GetCondition(vmcondition.TypeFilesystemFrozen, newVM.Status.Conditions)

	if oldFSFrozen.Reason != newFSFrozen.Reason {
		return true
	}

	oldSnapshotting, _ := conditions.GetCondition(vmcondition.TypeSnapshotting, oldVM.Status.Conditions)
	newSnapshotting, _ := conditions.GetCondition(vmcondition.TypeSnapshotting, newVM.Status.Conditions)

	return oldSnapshotting.Status != newSnapshotting.Status
}
