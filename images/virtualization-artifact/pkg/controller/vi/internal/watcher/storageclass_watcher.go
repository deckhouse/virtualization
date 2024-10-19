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

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StorageClassWatcher struct {
	client client.Client
	logger *slog.Logger
}

func NewStorageClassWatcher(client client.Client) *StorageClassWatcher {
	return &StorageClassWatcher{
		client: client,
		logger: slog.Default().With("watcher", strings.ToLower(virtv2.VirtualImageKind)),
	}
}

func (w StorageClassWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &storagev1.StorageClass{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool { return true },
			DeleteFunc: func(event event.DeleteEvent) bool { return true },
			UpdateFunc: func(event event.UpdateEvent) bool {
				oldSC, oldOk := event.ObjectOld.(*storagev1.StorageClass)
				newSC, newOk := event.ObjectNew.(*storagev1.StorageClass)
				if !oldOk || !newOk {
					return false
				}
				oldIsDefault, oldIsDefaultOk := oldSC.Annotations[common.AnnDefaultStorageClass]
				newIsDefault, newIsDefaultOk := newSC.Annotations[common.AnnDefaultStorageClass]
				switch {
				case oldIsDefaultOk && newIsDefaultOk:
					return oldIsDefault != newIsDefault
				case oldIsDefaultOk && !newIsDefaultOk:
					return oldIsDefault == "true"
				case !oldIsDefaultOk && newIsDefaultOk:
					return newIsDefault == "true"
				default:
					return false
				}
			},
		},
	)
}

func (w StorageClassWatcher) enqueueRequests(ctx context.Context, object client.Object) []reconcile.Request {
	sc, ok := object.(*storagev1.StorageClass)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected a Storage but got %T", object))
		return []reconcile.Request{}
	}

	var vis virtv2.VirtualImageList
	err := w.client.List(ctx, &vis, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVIByStorageClass, sc.Name),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list virtual images: %s", err))
		return []reconcile.Request{}
	}

	viMap := make(map[string]virtv2.VirtualImage, len(vis.Items))
	for _, vi := range vis.Items {
		viMap[vi.Name] = vi
	}

	vis.Items = []virtv2.VirtualImage{}

	isDefault, ok := sc.Annotations[common.AnnDefaultStorageClass]
	if ok && isDefault == "true" {
		err := w.client.List(ctx, &vis, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVIByStorageClass, indexer.DefaultStorageClass),
		})
		if err != nil {
			w.logger.Error(fmt.Sprintf("failed to list virtual images: %s", err))
			return []reconcile.Request{}
		}
	}

	for _, vi := range vis.Items {
		if _, ok := viMap[vi.Name]; !ok {
			viMap[vi.Name] = vi
		}
	}

	var requests []reconcile.Request
	for _, vi := range viMap {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vi.Name,
				Namespace: vi.Namespace,
			},
		})
	}

	return requests
}
