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

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
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
				// Only enqueue PCIDevice if NodePCIDevice has assignedNamespace
				if nodePCIDevice.Spec.AssignedNamespace == "" {
					return nil
				}

				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{
						Namespace: nodePCIDevice.Spec.AssignedNamespace,
						Name:      nodePCIDevice.Name,
					},
				}}
			}),
			predicate.TypedFuncs[*v1alpha2.NodePCIDevice]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.NodePCIDevice]) bool {
					return e.Object != nil
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.NodePCIDevice]) bool {
					return e.Object != nil
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.NodePCIDevice]) bool {
					return shouldProcessNodePCIDeviceUpdate(e.ObjectOld, e.ObjectNew)
				},
			},
		),
	)
}

func shouldProcessNodePCIDeviceUpdate(oldObj, newObj *v1alpha2.NodePCIDevice) bool {
	if oldObj == nil || newObj == nil {
		return false
	}

	return oldObj.Spec.AssignedNamespace != newObj.Spec.AssignedNamespace ||
		oldObj.Status.NodeName != newObj.Status.NodeName ||
		!equality.Semantic.DeepEqual(oldObj.Status.Attributes, newObj.Status.Attributes) ||
		!equality.Semantic.DeepEqual(oldObj.Status.Conditions, newObj.Status.Conditions) ||
		!equality.Semantic.DeepEqual(oldObj.GetDeletionTimestamp(), newObj.GetDeletionTimestamp())
}
