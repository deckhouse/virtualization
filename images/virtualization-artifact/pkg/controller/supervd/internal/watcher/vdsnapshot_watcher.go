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
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualDiskSnapshotWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewVirtualDiskSnapshotWatcher(client client.Client) *VirtualDiskSnapshotWatcher {
	return &VirtualDiskSnapshotWatcher{
		logger: log.Default().With("watcher", strings.ToLower(v1alpha2.VirtualDiskSnapshotKind)),
		client: client,
	}
}

func (w VirtualDiskSnapshotWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualDiskSnapshot{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualDiskSnapshot]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualDiskSnapshot]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDiskSnapshot: %w", err)
	}
	return nil
}

func (w VirtualDiskSnapshotWatcher) enqueueRequests(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (requests []reconcile.Request) {
	// 1. Need to reconcile the virtual disk from which the snapshot was taken.
	vd, err := object.FetchObject(ctx, types.NamespacedName{
		Name:      vdSnapshot.Spec.VirtualDiskName,
		Namespace: vdSnapshot.Namespace,
	}, w.client, &v1alpha2.VirtualDisk{})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to get virtual disk: %s", err))
		return
	}

	if vd != nil {
		if vd.Name == vdSnapshot.Spec.VirtualDiskName {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      vd.Name,
					Namespace: vd.Namespace,
				},
			})
		}
	}

	// Need to reconcile the virtual disk with the snapshot data source.
	var vds v1alpha2.VirtualDiskList
	err = w.client.List(ctx, &vds, &client.ListOptions{
		Namespace:     vdSnapshot.Namespace,
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVDByVDSnapshot, vdSnapshot.Name),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list virtual disks: %s", err))
		return
	}

	for _, vd := range vds.Items {
		if !isSnapshotDataSource(vd.Spec.DataSource, vdSnapshot.Name) {
			w.logger.Error("vd list by vd snapshot returns unexpected resources, please report a bug")
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vd.Name,
				Namespace: vd.Namespace,
			},
		})
	}

	return
}

func isSnapshotDataSource(ds *v1alpha2.VirtualDiskDataSource, vdSnapshotName string) bool {
	if ds == nil || ds.Type != v1alpha2.DataSourceTypeObjectRef {
		return false
	}

	if ds.ObjectRef == nil || ds.ObjectRef.Kind != v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot {
		return false
	}

	return ds.ObjectRef.Name == vdSnapshotName
}
