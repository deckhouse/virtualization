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

package watchers

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type ObjectRefWatcher struct {
	filter   UpdateEventsFilter
	enqueuer RequestEnqueuer
	logger   *log.Logger
}

type RequestEnqueuer interface {
	EnqueueRequests(context.Context, client.Object) []reconcile.Request
	GetEnqueueFrom() client.Object
}

type UpdateEventsFilter interface {
	FilterUpdateEvents(event.UpdateEvent) bool
}

func NewObjectRefWatcher(
	filter UpdateEventsFilter,
	enqueuer RequestEnqueuer,
) *ObjectRefWatcher {
	return &ObjectRefWatcher{
		filter:   filter,
		enqueuer: enqueuer,
		logger:   log.Default().With("watcher", "cvi"),
	}
}

func (w ObjectRefWatcher) Run(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), w.enqueuer.GetEnqueueFrom()),
		handler.EnqueueRequestsFromMapFunc(w.enqueuer.EnqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: w.filter.FilterUpdateEvents,
		},
	)
}
