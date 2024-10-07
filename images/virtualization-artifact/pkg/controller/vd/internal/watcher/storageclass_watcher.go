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

	storagev1 "k8s.io/api/storage/v1"
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

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StorageClassWatcher struct {
	logger *slog.Logger
	client client.Client
}

func NewStorageClassWatcher(client client.Client) *StorageClassWatcher {
	return &StorageClassWatcher{
		logger: slog.Default().With("watcher", virtv2.VirtualDiskKind),
		client: client,
	}
}

func (w StorageClassWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool { return true },
			DeleteFunc: func(event event.DeleteEvent) bool { return true },
			UpdateFunc: func(event event.UpdateEvent) bool { return false },
		},
	)
}

func (w StorageClassWatcher) enqueueRequests(ctx context.Context, object client.Object) []reconcile.Request {
	sc, ok := object.(*storagev1.StorageClass)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected a Storage but got %T", object))
	}

	var vds virtv2.VirtualDiskList
	err := w.client.List(ctx, &vds, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVDByStorageClass, sc.Name),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list virtual disks: %s", err))
		return []reconcile.Request{}
	}

	vdMap := make(map[string]virtv2.VirtualDisk)
	for _, vd := range vds.Items {
		vdMap[vd.Name] = vd
	}

	vds.Items = []virtv2.VirtualDisk{}

	_, isDefault := sc.Annotations[indexer.DefaultStorageClassAnnotation]
	if isDefault {
		err := w.client.List(ctx, &vds, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVDByStorageClass, indexer.DefaultStorageClass),
		})
		if err != nil {
			w.logger.Error(fmt.Sprintf("failed to list virtual disks: %s", err))
			return []reconcile.Request{}
		}
	}

	for _, vd := range vds.Items {
		if _, ok := vdMap[vd.Name]; !ok {
			vdMap[vd.Name] = vd
		}
	}

	var requests []reconcile.Request
	for _, vd := range vdMap {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vd.Name,
				Namespace: vd.Namespace,
			},
		})
	}

	return requests
}
