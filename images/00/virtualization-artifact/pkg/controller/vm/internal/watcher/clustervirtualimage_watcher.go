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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewClusterVirtualImageWatcher() *CLusterVirtualImageWatcher {
	return &CLusterVirtualImageWatcher{}
}

type CLusterVirtualImageWatcher struct{}

func (w *CLusterVirtualImageWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.ClusterVirtualImage{}),
		handler.EnqueueRequestsFromMapFunc(enqueueRequestsBlockDevice(mgr.GetClient(), virtv2.ClusterImageDevice)),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldCvi, oldOk := e.ObjectOld.(*virtv2.ClusterVirtualImage)
				newCvi, newOk := e.ObjectNew.(*virtv2.ClusterVirtualImage)
				if !oldOk || !newOk {
					return false
				}
				return oldCvi.Status.Phase != newCvi.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on ClusterVirtualImage: %w", err)
	}
	return nil
}
