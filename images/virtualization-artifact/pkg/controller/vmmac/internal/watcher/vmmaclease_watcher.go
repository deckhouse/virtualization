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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineMACAddressLeaseWatcher struct {
	client client.Client
	logger *log.Logger
}

func NewVirtualMachineMACAddressLeaseWatcher(client client.Client) *VirtualMachineMACAddressLeaseWatcher {
	return &VirtualMachineMACAddressLeaseWatcher{
		client: client,
		logger: log.Default().With("watcher", strings.ToLower(virtv2.VirtualMachineMACAddressLeaseKind)),
	}
}

func (w VirtualMachineMACAddressLeaseWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&virtv2.VirtualMachineMACAddressLease{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, lease *virtv2.VirtualMachineMACAddressLease) (requests []reconcile.Request) {
				vmmacRef := lease.Spec.VirtualMachineMACAddressRef
				if vmmacRef != nil {
					if vmmacRef.Name != "" && vmmacRef.Namespace != "" {
						return []reconcile.Request{
							{
								NamespacedName: types.NamespacedName{
									Namespace: vmmacRef.Namespace,
									Name:      vmmacRef.Name,
								},
							},
						}
					}
				}

				return
			}),
			predicate.TypedFuncs[*virtv2.VirtualMachineMACAddressLease]{
				CreateFunc: func(e event.TypedCreateEvent[*virtv2.VirtualMachineMACAddressLease]) bool {
					return true
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*virtv2.VirtualMachineMACAddressLease]) bool {
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualMachineMACAddressLease]) bool {
					return true
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineMACAddressLease: %w", err)
	}
	return nil
}
