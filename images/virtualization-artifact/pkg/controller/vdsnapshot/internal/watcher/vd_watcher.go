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
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w VirtualDiskWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	vd, ok := obj.(*virtv2.VirtualDisk)
	if !ok {
		slog.Default().Error(fmt.Sprintf("expected a VirtualDisk but got a %T", obj))
		return
	}

	var vdSnapshots virtv2.VirtualDiskSnapshotList
	err := w.client.List(ctx, &vdSnapshots, &client.ListOptions{
		Namespace: obj.GetNamespace(),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list vdsnapshots: %s", err))
		return
	}

	for _, vdSnapshot := range vdSnapshots.Items {
		if vdSnapshot.Spec.VirtualDiskName != vd.Name {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vdSnapshot.Name,
				Namespace: vdSnapshot.Namespace,
			},
		})
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

	if oldVD.Status.Phase != newVD.Status.Phase {
		return true
	}

	oldSnapshotting, _ := conditions.GetCondition(vdcondition.SnapshottingType, oldVD.Status.Conditions)
	newSnapshotting, _ := conditions.GetCondition(vdcondition.SnapshottingType, newVD.Status.Conditions)

	return oldSnapshotting.Status != newSnapshotting.Status || oldSnapshotting.Reason != newSnapshotting.Reason
}
