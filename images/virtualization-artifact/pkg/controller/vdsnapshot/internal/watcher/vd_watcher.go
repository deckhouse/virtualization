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
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv2.VirtualDisk]{
				UpdateFunc: w.filterUpdateEvents,
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
	}
	return nil
}

func (w VirtualDiskWatcher) enqueueRequests(ctx context.Context, vd *virtv2.VirtualDisk) (requests []reconcile.Request) {
	var vdSnapshots virtv2.VirtualDiskSnapshotList
	err := w.client.List(ctx, &vdSnapshots, &client.ListOptions{
		Namespace: vd.GetNamespace(),
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

func (w VirtualDiskWatcher) filterUpdateEvents(e event.TypedUpdateEvent[*virtv2.VirtualDisk]) bool {
	if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase {
		return true
	}

	oldSnapshotting, _ := conditions.GetCondition(vdcondition.SnapshottingType, e.ObjectOld.Status.Conditions)
	newSnapshotting, _ := conditions.GetCondition(vdcondition.SnapshottingType, e.ObjectNew.Status.Conditions)

	return oldSnapshotting.Status != newSnapshotting.Status || oldSnapshotting.Reason != newSnapshotting.Reason
}
