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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CVIWatcher struct {
	client client.Client
}

func NewCVIWatcher(client client.Client) *CVIWatcher {
	return &CVIWatcher{
		client: client,
	}
}

func (w *CVIWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &v1alpha2.ClusterVirtualImage{}),
		handler.EnqueueRequestsFromMapFunc(GetEnqueueForKind(w.client, v1alpha2.VirtualDataExportTargetClusterVirtualImage)),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldCVI, ok := e.ObjectOld.(*v1alpha2.ClusterVirtualImage)
				if !ok {
					return false
				}
				newCVI, ok := e.ObjectNew.(*v1alpha2.ClusterVirtualImage)
				if !ok {
					return false
				}

				return oldCVI.Status.Phase != newCVI.Status.Phase
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on ClusterVirtualImage: %w", err)
	}
	return nil
}
