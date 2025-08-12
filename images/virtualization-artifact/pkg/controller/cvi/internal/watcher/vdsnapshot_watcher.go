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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDiskSnapshot{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*virtv2.VirtualDiskSnapshot]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualDiskSnapshot]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDiskSnapshot: %w", err)
	}
	return nil
}

func (w VirtualDiskSnapshotWatcher) enqueueRequests(ctx context.Context, vdSnapshot *virtv2.VirtualDiskSnapshot) (requests []reconcile.Request) {
	var cvis virtv2.ClusterVirtualImageList
	err := w.client.List(ctx, &cvis, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldCVIByVDSnapshot, types.NamespacedName{
			Namespace: vdSnapshot.Namespace,
			Name:      vdSnapshot.Name,
		}.String()),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list cluster virtual images: %s", err))
		return
	}

	for _, cvi := range cvis.Items {
		if !isSnapshotDataSource(cvi.Spec.DataSource, vdSnapshot) {
			w.logger.Error("cvi list by vd snapshot returns unexpected resources, please report a bug")
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: cvi.Name,
			},
		})
	}

	return
}

func isSnapshotDataSource(ds virtv2.ClusterVirtualImageDataSource, vdSnapshot metav1.Object) bool {
	if ds.Type != virtv2.DataSourceTypeObjectRef {
		return false
	}

	if ds.ObjectRef == nil || ds.ObjectRef.Kind != virtv2.ClusterVirtualImageObjectRefKindVirtualDiskSnapshot {
		return false
	}

	return ds.ObjectRef.Name == vdSnapshot.GetName() && ds.ObjectRef.Namespace == vdSnapshot.GetNamespace()
}
