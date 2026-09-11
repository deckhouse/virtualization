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

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PCIDeviceWatcher struct {
	client client.Client
}

func NewPCIDeviceWatcher(client client.Client) *PCIDeviceWatcher {
	return &PCIDeviceWatcher{
		client: client,
	}
}

func (w *PCIDeviceWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.PCIDevice{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueue),
		),
	)
}

func (w *PCIDeviceWatcher) enqueue(ctx context.Context, pciDevice *v1alpha2.PCIDevice) []reconcile.Request {
	var vms v1alpha2.VirtualMachineList
	err := w.client.List(ctx, &vms, &client.ListOptions{
		Namespace:     pciDevice.Namespace,
		FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMByPCIDevice, pciDevice.Name),
	})
	if err != nil {
		return nil
	}

	var result []reconcile.Request
	for _, vm := range vms.Items {
		result = append(result, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vm.GetName(),
				Namespace: vm.GetNamespace(),
			},
		})
	}
	return result
}
