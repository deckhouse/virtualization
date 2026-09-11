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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewNodePCIDeviceWatcher() *NodePCIDeviceWatcher {
	return &NodePCIDeviceWatcher{}
}

type NodePCIDeviceWatcher struct{}

func (w *NodePCIDeviceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&v1alpha2.NodePCIDevice{},
			handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, nodePCIDevice *v1alpha2.NodePCIDevice) []reconcile.Request {
				return requestsByNodePCIDeviceDeletion(nodePCIDevice)
			}),
			predicate.TypedFuncs[*v1alpha2.NodePCIDevice]{
				CreateFunc: func(_ event.TypedCreateEvent[*v1alpha2.NodePCIDevice]) bool {
					return false
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.NodePCIDevice]) bool {
					return e.Object != nil
				},
				UpdateFunc: func(_ event.TypedUpdateEvent[*v1alpha2.NodePCIDevice]) bool {
					return false
				},
			},
		),
	)
}

func requestsByNodePCIDeviceDeletion(nodePCIDevice *v1alpha2.NodePCIDevice) []reconcile.Request {
	if nodePCIDevice == nil {
		return nil
	}

	for _, ownerReference := range nodePCIDevice.OwnerReferences {
		if ownerReference.Kind != "ResourceSlice" || ownerReference.Name == "" {
			continue
		}

		return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: ownerReference.Name}}}
	}

	return nil
}
