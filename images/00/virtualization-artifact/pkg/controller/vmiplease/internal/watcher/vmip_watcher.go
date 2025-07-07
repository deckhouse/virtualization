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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineIPAddressWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewVirtualMachineIPAddressWatcher(client client.Client) *VirtualMachineIPAddressWatcher {
	return &VirtualMachineIPAddressWatcher{
		logger: log.Default().With("watcher", strings.ToLower(virtv2.VirtualMachineIPAddressKind)),
		client: client,
	}
}

func (w VirtualMachineIPAddressWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddress{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	)
}

func (w VirtualMachineIPAddressWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	vmip, ok := obj.(*virtv2.VirtualMachineIPAddress)
	if !ok {
		return nil
	}

	var leases virtv2.VirtualMachineIPAddressLeaseList
	err := w.client.List(ctx, &leases, &client.ListOptions{})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list leases: %s", err))
		return
	}

	for _, lease := range leases.Items {
		if vmip.Status.Address != "" && vmip.Status.Address == ip.LeaseNameToIP(lease.Name) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: lease.Name},
			})
			continue
		}

		vmipRef := lease.Spec.VirtualMachineIPAddressRef
		if vmipRef != nil && vmipRef.Name == obj.GetName() && vmipRef.Namespace == obj.GetNamespace() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: lease.Name},
			})
		}
	}

	return requests
}
