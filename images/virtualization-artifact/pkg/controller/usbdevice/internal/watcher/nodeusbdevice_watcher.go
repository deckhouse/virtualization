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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewNodeUSBDeviceWatcher() *NodeUSBDeviceWatcher {
	return &NodeUSBDeviceWatcher{}
}

type NodeUSBDeviceWatcher struct{}

func (w *NodeUSBDeviceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&v1alpha2.NodeUSBDevice{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, nodeUSBDevice *v1alpha2.NodeUSBDevice) []reconcile.Request {
				var result []reconcile.Request

				// Only enqueue USBDevice if NodeUSBDevice has assignedNamespace
				if nodeUSBDevice.Spec.AssignedNamespace == "" {
					return nil
				}

				// USBDevice has the same name as NodeUSBDevice and is in the assignedNamespace
				usbDevice := &v1alpha2.USBDevice{}
				key := types.NamespacedName{
					Namespace: nodeUSBDevice.Spec.AssignedNamespace,
					Name:      nodeUSBDevice.Name,
				}
				if err := mgr.GetClient().Get(ctx, key, usbDevice); err != nil {
					// USBDevice doesn't exist yet - it will be created by the assigned handler
					if errors.IsNotFound(err) {
						return nil
					}

					log.Error("failed to get USBDevice", "error", err, "usbDevice", usbDevice)

					return nil
				}

				result = append(result, reconcile.Request{
					NamespacedName: object.NamespacedName(usbDevice),
				})

				return result
			}),
		),
	)
}
