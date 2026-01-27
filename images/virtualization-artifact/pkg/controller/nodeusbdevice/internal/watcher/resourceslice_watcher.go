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
	"strings"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
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

				// Check if ResourceSlice contains USB devices
				hasUSBDevices := false
				for _, device := range slice.Spec.Devices {
					if strings.HasPrefix(device.Name, "virtualization-dra-") {
						hasUSBDevices = true
						break
					}
				}

				// If no USB devices in this ResourceSlice, skip reconciliation
				if !hasUSBDevices {
					return nil
				}

				var result []reconcile.Request
				client := mgr.GetClient()

				// Enqueue all existing NodeUSBDevices for reconciliation
				deviceList := &v1alpha2.NodeUSBDeviceList{}
				if err := client.List(ctx, deviceList); err != nil {
					log.Error("failed to list NodeUSBDevices in ResourceSliceWatcher", log.Err(err))
					return nil
				}

				hasDevicesOnNode := false
				for _, device := range deviceList.Items {
					// Only enqueue devices from the same node as the ResourceSlice
					if device.Status.NodeName == slice.Spec.Pool.Name {
						hasDevicesOnNode = true
						result = append(result, reconcile.Request{
							NamespacedName: object.NamespacedName(&device),
						})
					}
				}

				// If no devices exist on this node yet, trigger discovery by enqueueing
				// any existing NodeUSBDevice to trigger a reconciliation cycle.
				// DiscoveryHandler will check all ResourceSlices during reconciliation
				// and automatically create new NodeUSBDevice for devices found in this ResourceSlice.
				if !hasDevicesOnNode && len(deviceList.Items) > 0 {
					// Enqueue first device to trigger reconciliation cycle
					// DiscoveryHandler will discover new devices during this cycle
					result = append(result, reconcile.Request{
						NamespacedName: object.NamespacedName(&deviceList.Items[0]),
					})
				}

				// Note: If no NodeUSBDevices exist at all, discovery will happen
				// on next periodic reconciliation or when controller starts

				return result
			}),
		),
	)
}
