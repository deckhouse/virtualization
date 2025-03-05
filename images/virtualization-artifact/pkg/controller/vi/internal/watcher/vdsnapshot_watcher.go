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
	"strings"

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

type VirtualDiskSnapshotWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewVirtualDiskSnapshotWatcher(client client.Client) *VirtualDiskSnapshotWatcher {
	return &VirtualDiskSnapshotWatcher{
		logger: log.Default().With("watcher", strings.ToLower(virtv2.VirtualDiskSnapshotKind)),
		client: client,
	}
}

func (w VirtualDiskSnapshotWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDiskSnapshot{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w VirtualDiskSnapshotWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	vdSnapshot, ok := obj.(*virtv2.VirtualDiskSnapshot)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected a VirtualDiskSnapshot but got a %T", obj))
		return
	}

	var vis virtv2.VirtualImageList
	err := w.client.List(ctx, &vis, &client.ListOptions{
		Namespace:     vdSnapshot.Namespace,
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVIByVDSnapshot, vdSnapshot.Name),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list virtual images: %s", err))
		return
	}

	for _, vi := range vis.Items {
		if !isSnapshotDataSource(vi.Spec.DataSource, vdSnapshot.Name) {
			w.logger.Error("vi list by vd snapshot returns unexpected resources, please report a bug")
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vi.Name,
				Namespace: vi.Namespace,
			},
		})
	}

	return
}

func (w VirtualDiskSnapshotWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldVDSnapshot, ok := e.ObjectOld.(*virtv2.VirtualDiskSnapshot)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected an old VirtualDiskSnapshot but got a %T", e.ObjectOld))
		return false
	}

	newVDSnapshot, ok := e.ObjectNew.(*virtv2.VirtualDiskSnapshot)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected a new VirtualDiskSnapshot but got a %T", e.ObjectNew))
		return false
	}

	return oldVDSnapshot.Status.Phase != newVDSnapshot.Status.Phase
}

func isSnapshotDataSource(ds virtv2.VirtualImageDataSource, vdSnapshotName string) bool {
	if ds.Type != virtv2.DataSourceTypeObjectRef {
		return false
	}

	if ds.ObjectRef == nil || ds.ObjectRef.Kind != virtv2.VirtualImageObjectRefKindVirtualDiskSnapshot {
		return false
	}

	return ds.ObjectRef.Name == vdSnapshotName
}
