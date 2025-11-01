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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StorageClassWatcher struct {
	client client.Client
	logger *slog.Logger
}

func NewStorageClassWatcher(client client.Client) *StorageClassWatcher {
	return &StorageClassWatcher{
		client: client,
		logger: slog.Default().With("watcher", strings.ToLower(v1alpha2.VirtualDiskKind)),
	}
}

func (w StorageClassWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &storagev1.StorageClass{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*storagev1.StorageClass]{
				UpdateFunc: func(e event.TypedUpdateEvent[*storagev1.StorageClass]) bool {
					oldIsDefault, oldIsDefaultOk := e.ObjectOld.Annotations[annotations.AnnDefaultStorageClass]
					newIsDefault, newIsDefaultOk := e.ObjectNew.Annotations[annotations.AnnDefaultStorageClass]
					switch {
					case oldIsDefaultOk && newIsDefaultOk:
						return oldIsDefault != newIsDefault
					case !oldIsDefaultOk && newIsDefaultOk:
						return newIsDefault == "true"
					default:
						return false
					}
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on StorageClass: %w", err)
	}

	return nil
}

func (w StorageClassWatcher) enqueueRequests(ctx context.Context, sc *storagev1.StorageClass) []reconcile.Request {
	var selectorValue string
	if isDefault, ok := sc.Annotations[annotations.AnnDefaultStorageClass]; ok && isDefault == "true" {
		selectorValue = indexer.DefaultStorageClass
	} else {
		selectorValue = sc.Name
	}

	fieldSelector := fields.OneTermEqualSelector(indexer.IndexFieldVDByStorageClass, selectorValue)

	var vds v1alpha2.VirtualDiskList
	err := w.client.List(ctx, &vds, &client.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list virtual disks: %v", err))
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, vd := range vds.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vd.Name,
				Namespace: vd.Namespace,
			},
		})
	}

	return requests
}
