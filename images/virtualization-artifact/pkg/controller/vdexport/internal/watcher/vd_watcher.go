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

type VDWatcher struct {
	dataExportEnabled bool
	client            client.Client
}

func NewVDWatcher(dataExportEnabled bool, client client.Client) *VDWatcher {
	return &VDWatcher{
		dataExportEnabled: dataExportEnabled,
		client:            client,
	}
}

func (w *VDWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if !w.dataExportEnabled {
		return nil
	}

	if err := ctr.Watch(source.Kind(mgr.GetCache(), &v1alpha2.VirtualDisk{}),
		handler.EnqueueRequestsFromMapFunc(GetEnqueueForKind(w.client, v1alpha2.VirtualDataExportTargetVirtualDisk)),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVD, ok := e.ObjectOld.(*v1alpha2.VirtualDisk)
				if !ok {
					return false
				}
				newVD, ok := e.ObjectNew.(*v1alpha2.VirtualDisk)
				if !ok {
					return false
				}

				return oldVD.Status.Phase != newVD.Status.Phase
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
	}
	return nil
}
