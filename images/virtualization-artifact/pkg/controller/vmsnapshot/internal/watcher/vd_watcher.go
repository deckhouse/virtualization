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
	"strings"

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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
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
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return false },
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w VirtualDiskWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var vmSnapshots virtv2.VirtualMachineSnapshotList
	err := w.client.List(ctx, &vmSnapshots, &client.ListOptions{
		Namespace: obj.GetNamespace(),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list virtual machine snapshots: %s", err))
		return
	}

	for _, vmSnapshot := range vmSnapshots.Items {
		for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
			vdName, ok := strings.CutSuffix(vdSnapshotName, "-"+string(vmSnapshot.UID))
			if !ok {
				slog.Default().Error("Failed to get virtual disk name from virtual disk snapshot name, please report a bug")
				continue
			}

			if vdName == obj.GetName() {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      vmSnapshot.Name,
						Namespace: vmSnapshot.Namespace,
					},
				})
				break
			}
		}
	}

	return
}

func (w VirtualDiskWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldVD, ok := e.ObjectOld.(*virtv2.VirtualDisk)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected an old VirtualDisk but got a %T", e.ObjectOld))
		return false
	}

	newVD, ok := e.ObjectNew.(*virtv2.VirtualDisk)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected a new VirtualDisk but got a %T", e.ObjectNew))
		return false
	}

	oldResized, _ := conditions.GetCondition(vdcondition.ResizingType, oldVD.Status.Conditions)
	newResized, _ := conditions.GetCondition(vdcondition.ResizingType, newVD.Status.Conditions)

	return oldResized.Status != newResized.Status || oldResized.Reason != newResized.Reason
}
