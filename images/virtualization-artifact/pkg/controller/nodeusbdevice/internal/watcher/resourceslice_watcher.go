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

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	draDriverName = "virtualization-dra"
)

func NewResourceSliceWatcher() *ResourceSliceWatcher {
	return &ResourceSliceWatcher{}
}

type ResourceSliceWatcher struct{}

func (w *ResourceSliceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&resourcev1beta1.ResourceSlice{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, slice *resourcev1beta1.ResourceSlice) []reconcile.Request {
				// Only watch ResourceSlices from virtualization-dra driver
				if slice.Spec.Driver != draDriverName {
					return nil
				}

				var result []reconcile.Request

				// Enqueue all existing NodeUSBDevices for reconciliation
				deviceList := &v1alpha2.NodeUSBDeviceList{}
				if err := mgr.GetClient().List(ctx, deviceList); err != nil {
					return nil
				}

				for _, device := range deviceList.Items {
					// Only enqueue devices from the same node as the ResourceSlice
					if device.Status.NodeName == slice.Spec.Pool.Name {
						result = append(result, reconcile.Request{
							NamespacedName: object.NamespacedName(&device),
						})
					}
				}

				// Also trigger discovery to create new NodeUSBDevices
				// This is done by enqueueing a special request that will trigger discovery
				// For now, we'll rely on periodic reconciliation or manual creation
				// TODO: Implement automatic creation of NodeUSBDevice from ResourceSlice

				return result
			}),
		),
	)
}
