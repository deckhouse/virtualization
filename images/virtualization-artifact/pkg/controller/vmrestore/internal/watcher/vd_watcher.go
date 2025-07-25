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

	"github.com/deckhouse/deckhouse/pkg/log"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualDiskWatcher struct {
	client client.Client
}

func NewVirtualDiskWatcher(client client.Client) *VirtualDiskWatcher {
	return &VirtualDiskWatcher{
		client: client,
	}
}

func (w VirtualDiskWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	)
}

func (w VirtualDiskWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	vd, ok := obj.(*virtv2.VirtualDisk)
	if !ok {
		log.Error(fmt.Sprintf("expected a VirtualDisk but got a %T", obj))
		return
	}

	var vmRestores virtv2.VirtualMachineRestoreList
	err := w.client.List(ctx, &vmRestores, &client.ListOptions{
		Namespace: obj.GetNamespace(),
	})
	if err != nil {
		log.Error(fmt.Sprintf("failed to list vmRestores: %s", err))
		return
	}

	for _, vmRestore := range vmRestores.Items {
		vmSnapshotName := vmRestore.Spec.VirtualMachineSnapshotName
		var vmSnapshot virtv2.VirtualMachineSnapshot
		err := w.client.Get(ctx, types.NamespacedName{Name: vmSnapshotName, Namespace: obj.GetNamespace()}, &vmSnapshot)
		if err != nil {
			log.Error(fmt.Sprintf("failed to get vmSnapshot: %s", err))
			return
		}
		for _, vdsnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
			var vdSnapshot virtv2.VirtualDiskSnapshot
			err := w.client.Get(ctx, types.NamespacedName{Name: vdsnapshotName, Namespace: obj.GetNamespace()}, &vdSnapshot)
			if err != nil {
				log.Error(fmt.Sprintf("failed to get vdSnapshot: %s", err))
				return
			}

			if w.isVdNameMatch(vd.Name, vdSnapshot.Spec.VirtualDiskName, vmRestore.Spec.NameReplacements) {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      vmRestore.Name,
						Namespace: vmRestore.Namespace,
					},
				})
			}
		}
	}

	return
}

func (w VirtualDiskWatcher) isVdNameMatch(vdName, restoredName string, nameReplacements []virtv2.NameReplacement) bool {
	var (
		isNameMatch            bool
		isNameReplacementMatch bool
	)

	isNameMatch = vdName == restoredName

	for _, nr := range nameReplacements {
		if nr.From.Kind != virtv2.VirtualDiskKind {
			continue
		}

		if nr.From.Name == vdName {
			isNameReplacementMatch = true
			break
		}
	}

	return isNameMatch || isNameReplacementMatch
}
