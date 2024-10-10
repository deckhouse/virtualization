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

type ClusterVirtualImageWatcher struct {
	logger *slog.Logger
}

func NewClusterVirtualImageWatcher() *ClusterVirtualImageWatcher {
	return &ClusterVirtualImageWatcher{
		logger: slog.Default().With("watcher", strings.ToLower(virtv2.ClusterVirtualImageKind)),
	}
}

func (w ClusterVirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: w.filterUpdateEvents,
		},
	)
}

func (w ClusterVirtualImageWatcher) enqueueRequests(_ context.Context, obj client.Object) (requests []reconcile.Request) {
	cvi, ok := obj.(*virtv2.ClusterVirtualImage)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected a VirtualImage but got a %T", obj))
		return
	}

	if cvi.Spec.DataSource.Type == virtv2.DataSourceTypeObjectRef && cvi.Spec.DataSource.ObjectRef != nil && cvi.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualDiskKind {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      cvi.Spec.DataSource.ObjectRef.Name,
				Namespace: cvi.Spec.DataSource.ObjectRef.Namespace,
			},
		})
	}

	return
}

func (w ClusterVirtualImageWatcher) filterUpdateEvents(e event.UpdateEvent) bool {
	oldCVI, ok := e.ObjectOld.(*virtv2.ClusterVirtualImage)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected an old VirtualImage but got a %T", e.ObjectOld))
		return false
	}

	newCVI, ok := e.ObjectNew.(*virtv2.ClusterVirtualImage)
	if !ok {
		w.logger.Error(fmt.Sprintf("expected a new VirtualImage but got a %T", e.ObjectNew))
		return false
	}

	return oldCVI.Status.Phase != newCVI.Status.Phase
}
