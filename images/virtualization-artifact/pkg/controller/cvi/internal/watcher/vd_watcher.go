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

type VirtualDiskWatcher struct {
	client client.Client
}

func NewVirtualDiskWatcher(client client.Client) *VirtualDiskWatcher {
	return &VirtualDiskWatcher{
		client: client,
	}
}

func (w VirtualDiskWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return isDataSourceCVI(e.Object)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return isDataSourceCVI(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return isDataSourceCVI(e.ObjectOld) || isDataSourceCVI(e.ObjectNew)
			},
		},
	)
}

func (w VirtualDiskWatcher) enqueueRequests(_ context.Context, obj client.Object) (requests []reconcile.Request) {
	vd, ok := obj.(*virtv2.VirtualDisk)
	if !ok {
		return
	}

	if !isDataSourceCVI(vd) {
		return
	}

	requests = append(requests, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: vd.Spec.DataSource.ObjectRef.Name,
		},
	})

	return
}
