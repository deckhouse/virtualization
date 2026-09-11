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

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
)

const (
	draDriverName       = "virtualization-pci"
	pciDeviceNamePrefix = "pci-"
)

func NewResourceSliceWatcher() *ResourceSliceWatcher {
	return &ResourceSliceWatcher{}
}

type ResourceSliceWatcher struct{}

func (w *ResourceSliceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(),
			&resourcev1.ResourceSlice{},
			handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, resourceSlice *resourcev1.ResourceSlice) []reconcile.Request {
				return requestsByResourceSlice(resourceSlice)
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

func requestsByResourceSlice(resourceSlice *resourcev1.ResourceSlice) []reconcile.Request {
	if resourceSlice == nil || resourceSlice.Spec.Driver != draDriverName {
		log.Error("resource slice is not a DRA slice")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(resourceSlice.Spec.Devices))
	seen := make(map[types.NamespacedName]struct{}, len(resourceSlice.Spec.Devices))

	for _, nodePCIDeviceName := range nodePCIDeviceNamesFromSlice(resourceSlice) {
		key := types.NamespacedName{Name: nodePCIDeviceName}
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		requests = append(requests, reconcile.Request{NamespacedName: key})
	}

	return requests
}

func nodePCIDeviceNamesFromSlice(resourceSlice *resourcev1.ResourceSlice) []string {
	result := make([]string, 0, len(resourceSlice.Spec.Devices))
	seen := make(map[string]struct{}, len(resourceSlice.Spec.Devices))

	for _, device := range resourceSlice.Spec.Devices {
		nodePCIDeviceName, ok := nodePCIDeviceName(device)
		if !ok {
			continue
		}

		if _, exists := seen[nodePCIDeviceName]; exists {
			continue
		}

		seen[nodePCIDeviceName] = struct{}{}
		result = append(result, nodePCIDeviceName)
	}

	return result
}

func nodePCIDeviceName(device resourcev1.Device) (string, bool) {
	if !strings.HasPrefix(device.Name, pciDeviceNamePrefix) {
		return "", false
	}

	name := device.Name
	if attr, ok := device.Attributes["name"]; ok && attr.StringValue != nil && *attr.StringValue != "" {
		name = *attr.StringValue
	}

	sanitizedName := strings.ToLower(strings.ReplaceAll(name, ".", "-"))
	if sanitizedName == "" {
		return "", false
	}

	return sanitizedName, true
}
