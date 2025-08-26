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
	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineMACAddressWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewVirtualMachineMACAddressWatcher(client client.Client) *VirtualMachineMACAddressWatcher {
	return &VirtualMachineMACAddressWatcher{
		logger: log.Default().With("watcher", strings.ToLower(virtv2.VirtualMachineMACAddressKind)),
		client: client,
	}
}

func (w *VirtualMachineMACAddressWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&virtv2.VirtualMachineMACAddress{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vmmac *virtv2.VirtualMachineMACAddress) []reconcile.Request {
				var requests []reconcile.Request

				var leases virtv2.VirtualMachineMACAddressLeaseList
				if err := w.client.List(ctx, &leases, &client.ListOptions{}); err != nil {
					w.logger.Error(fmt.Sprintf("failed to list leases: %s", err))
					return nil
				}

				for _, lease := range leases.Items {
					if vmmac.Status.Address != "" && vmmac.Status.Address == mac.LeaseNameToAddress(lease.Name) {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{Name: lease.Name},
						})
						continue
					}
				}

				return requests
			}),
			predicate.TypedFuncs[*virtv2.VirtualMachineMACAddress]{
				CreateFunc: func(e event.TypedCreateEvent[*virtv2.VirtualMachineMACAddress]) bool {
					return false
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*virtv2.VirtualMachineMACAddress]) bool {
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualMachineMACAddress]) bool {
					return true
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineMACAddress: %w", err)
	}
	return nil
}
