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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VirtualMachineSnapshotWatcher wakes the children of a virtual machine snapshot up when its phase changes,
// so a child takes its consistency from the parent once the parent is terminal.
type VirtualMachineSnapshotWatcher struct{}

func NewVirtualMachineSnapshotWatcher() *VirtualMachineSnapshotWatcher {
	return &VirtualMachineSnapshotWatcher{}
}

func (w VirtualMachineSnapshotWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineSnapshot{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineSnapshot]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineSnapshot]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineSnapshot: %w", err)
	}

	return nil
}

func (w VirtualMachineSnapshotWatcher) enqueueRequests(_ context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot) []reconcile.Request {
	requests := make([]reconcile.Request, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))
	for _, name := range vmSnapshot.Status.VirtualDiskSnapshotNames {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: name}})
	}

	return requests
}
