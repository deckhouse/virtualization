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

func NewPCIDeviceWatcher() *PCIDeviceWatcher {
	return &PCIDeviceWatcher{}
}

type PCIDeviceWatcher struct{}

func (w *PCIDeviceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&v1alpha2.PCIDevice{},
			handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, pciDevice *v1alpha2.PCIDevice) []reconcile.Request {
				return requestsByPCIDevice(pciDevice)
			}),
			predicate.TypedFuncs[*v1alpha2.PCIDevice]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.PCIDevice]) bool {
					return shouldProcessPCIDeviceUpdate(e.ObjectOld, e.ObjectNew)
				},
			},
		),
	)
}

func requestsByPCIDevice(pciDevice *v1alpha2.PCIDevice) []reconcile.Request {
	if pciDevice == nil {
		return nil
	}

	for _, ownerRef := range pciDevice.OwnerReferences {
		if ownerRef.APIVersion != v1alpha2.SchemeGroupVersion.String() || ownerRef.Kind != v1alpha2.NodePCIDeviceKind || ownerRef.Name == "" {
			continue
		}

		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: ownerRef.Name}}}
	}

	if pciDevice.Name == "" {
		return nil
	}

	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: pciDevice.Name}}}
}

func shouldProcessPCIDeviceUpdate(oldObj, newObj *v1alpha2.PCIDevice) bool {
	if oldObj == nil || newObj == nil {
		return false
	}

	return !equality.Semantic.DeepEqual(oldObj.Status.Conditions, newObj.Status.Conditions) ||
		!equality.Semantic.DeepEqual(oldObj.OwnerReferences, newObj.OwnerReferences) ||
		!equality.Semantic.DeepEqual(oldObj.GetDeletionTimestamp(), newObj.GetDeletionTimestamp())
}
