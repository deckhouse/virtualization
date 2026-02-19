/*
Copyright 2026 Flant JSC

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

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	draDriverName = "virtualization-usb"
)

func NewResourceSliceWatcher() *ResourceSliceWatcher {
	return &ResourceSliceWatcher{}
}

type ResourceSliceWatcher struct{}

func (w *ResourceSliceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&resourcev1.ResourceSlice{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, slice *resourcev1.ResourceSlice) []reconcile.Request {
				_ = ctx

				return []reconcile.Request{{
					NamespacedName: client.ObjectKeyFromObject(slice),
				}}
			}),
			predicate.TypedFuncs[*resourcev1.ResourceSlice]{
				CreateFunc: func(e event.TypedCreateEvent[*resourcev1.ResourceSlice]) bool {
					return e.Object != nil && e.Object.Spec.Driver == draDriverName
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*resourcev1.ResourceSlice]) bool {
					return e.Object != nil && e.Object.Spec.Driver == draDriverName
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*resourcev1.ResourceSlice]) bool {
					if e.ObjectOld == nil || e.ObjectNew == nil {
						return false
					}

					return e.ObjectOld.Spec.Driver == draDriverName || e.ObjectNew.Spec.Driver == draDriverName
				},
			},
		),
	)
}
