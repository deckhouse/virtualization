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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualImageWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewVirtualImageWatcher(client client.Client) *VirtualImageWatcher {
	return &VirtualImageWatcher{
		logger: log.Default().With("watcher", "vi"),
		client: client,
	}
}

func (w VirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return w.isDataSourceVI(e.Object)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return w.isDataSourceVI(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return w.isDataSourceVI(e.ObjectOld) || w.isDataSourceVI(e.ObjectNew)
			},
		},
	)
}

func (w VirtualImageWatcher) enqueueRequests(_ context.Context, obj client.Object) (requests []reconcile.Request) {
	vi, ok := obj.(*virtv2.VirtualImage)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected a VirtualImage but got a %T", obj))
		return
	}

	if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef {
		return
	}

	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualImageKind {
		return
	}

	requests = append(requests, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      vi.Spec.DataSource.ObjectRef.Name,
			Namespace: vi.Namespace,
		},
	})

	return
}

func (w VirtualImageWatcher) isDataSourceVI(obj client.Object) bool {
	vi, ok := obj.(*virtv2.VirtualImage)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected a VirtualImage but got a %T", obj))
		return false
	}

	return vi.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef && vi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualImageKind
}
