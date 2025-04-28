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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineSnapshotWatcher struct{}

func NewVirtualMachineSnapshotWatcher() *VirtualMachineSnapshotWatcher {
	return &VirtualMachineSnapshotWatcher{}
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

func (w VirtualMachineSnapshotWatcher) enqueueRequests(_ context.Context, obj client.Object) (requests []reconcile.Request) {
	vmSnapshot, ok := obj.(*virtv2.VirtualMachineSnapshot)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected a VirtualMachineSnapshot but got a %T", obj))
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      vmSnapshot.Spec.VirtualMachineName,
				Namespace: vmSnapshot.Namespace,
			},
		},
	}
}

func (w VirtualMachineSnapshotWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldVMSnapshot, ok := e.ObjectOld.(*virtv2.VirtualMachineSnapshot)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected an old VirtualMachineSnapshot but got a %T", e.ObjectOld))
		return false
	}

	newVMSnapshot, ok := e.ObjectNew.(*virtv2.VirtualMachineSnapshot)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected a new VirtualMachineSnapshot but got a %T", e.ObjectNew))
		return false
	}

	return oldVMSnapshot.Status.Phase != newVMSnapshot.Status.Phase
}
