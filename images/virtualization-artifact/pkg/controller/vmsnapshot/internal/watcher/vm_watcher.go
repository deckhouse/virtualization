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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachine]) bool { return false },
				UpdateFunc: w.filterUpdateEvents,
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}

func (w VirtualMachineWatcher) enqueueRequests(ctx context.Context, vm *v1alpha2.VirtualMachine) (requests []reconcile.Request) {
	var vmSnapshots v1alpha2.VirtualMachineSnapshotList
	err := w.client.List(ctx, &vmSnapshots, &client.ListOptions{
		Namespace:     vm.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMSnapshotByVM, vm.GetName()),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list virtual machine snapshots: %s", err))
		return
	}

	for _, vmSnapshot := range vmSnapshots.Items {
		if vmSnapshot.Spec.VirtualMachineName == vm.GetName() {
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

func (w VirtualMachineWatcher) filterUpdateEvents(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
	oldAgentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, e.ObjectOld.Status.Conditions)
	newAgentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, e.ObjectNew.Status.Conditions)

	if oldAgentReady != newAgentReady {
		return true
	}

	oldSnapshotting, _ := conditions.GetCondition(vmcondition.TypeSnapshotting, e.ObjectOld.Status.Conditions)
	newSnapshotting, _ := conditions.GetCondition(vmcondition.TypeSnapshotting, e.ObjectNew.Status.Conditions)

	return oldSnapshotting.Status != newSnapshotting.Status
}
