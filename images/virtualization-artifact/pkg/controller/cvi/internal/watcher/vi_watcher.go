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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualImageWatcher struct {
	client client.Client
}

func NewVirtualImageWatcher(client client.Client) *VirtualImageWatcher {
	return &VirtualImageWatcher{
		client: client,
	}
}

func (w VirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return w.isDataSourceCVI(e.Object)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return w.isDataSourceCVI(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return w.isDataSourceCVI(e.ObjectOld) || w.isDataSourceCVI(e.ObjectNew)
			},
		},
	)
}

func (w VirtualImageWatcher) enqueueRequests(_ context.Context, obj client.Object) (requests []reconcile.Request) {
	vi, ok := obj.(*virtv2.VirtualImage)
	if !ok {
		return
	}

	if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef {
		return
	}

	if vi.Spec.DataSource.ObjectRef == nil || vi.Spec.DataSource.ObjectRef.Kind != virtv2.ClusterVirtualImageKind {
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

func (w VirtualImageWatcher) isDataSourceCVI(obj client.Object) bool {
	vi, ok := obj.(*virtv2.VirtualImage)
	if !ok {
		return false
	}

	return vi.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef && vi.Spec.DataSource.ObjectRef.Kind == virtv2.ClusterVirtualImageKind
}
