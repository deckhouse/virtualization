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
	"log/slog"

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
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ModuleConfigWatcher struct {
	client client.Client
	logger *slog.Logger
}

func NewModuleConfigWatcher(client client.Client) *StorageClassWatcher {
	return &StorageClassWatcher{
		client: client,
		logger: slog.Default().With("watcher", "moduleconfig"),
	}
}

func (w ModuleConfigWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &mcapi.ModuleConfig{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool { return true },
			DeleteFunc: func(event event.DeleteEvent) bool { return true },
			UpdateFunc: func(event event.UpdateEvent) bool {
				oldMc, oldOk := event.ObjectOld.(*mcapi.ModuleConfig)
				newMc, newOk := event.ObjectNew.(*mcapi.ModuleConfig)
				if !oldOk || !newOk {
					return false
				}
				var (
					oldViDefaultSc string
					newViDefaultSc string
				)
				if virtualImages, ok := oldMc.Spec.Settings["virtualImages"].(map[string]interface{}); ok {
					if defaultClass, ok := virtualImages["defaultStorageClassName"].(string); ok {
						oldViDefaultSc = defaultClass
					}
				}
				if virtualImages, ok := newMc.Spec.Settings["virtualImages"].(map[string]interface{}); ok {
					if defaultClass, ok := virtualImages["defaultStorageClassName"].(string); ok {
						oldViDefaultSc = defaultClass
					}
				}
				return oldViDefaultSc != newViDefaultSc
			},
		},
	)
}

func (w ModuleConfigWatcher) enqueueRequests(ctx context.Context, object client.Object) []reconcile.Request {
	var vis virtv2.VirtualImageList
	err := w.client.List(ctx, &vis, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVIByNotReadyStorageClass, string(vicondition.StorageClassReadyType)),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list virtual images: %s", err))
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, vi := range vis.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vi.Name,
				Namespace: vi.Namespace,
			},
		})
	}

	return requests
}
