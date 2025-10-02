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

type VirtualMachineMACAddressLeaseWatcher struct{}

func NewVirtualMachineMACAddressLeaseWatcher() *VirtualMachineMACAddressLeaseWatcher {
	return &VirtualMachineMACAddressLeaseWatcher{}
}

func (w *VirtualMachineMACAddressLeaseWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachineMACAddressLease{},
			handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, lease *v1alpha2.VirtualMachineMACAddressLease) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: lease.Namespace,
							Name:      lease.Name,
						},
					},
				}
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineMACAddressLease]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachineMACAddressLease]) bool {
					return true
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachineMACAddressLease]) bool {
					return false
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineMACAddressLease]) bool {
					return true
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineMACAddressLease: %w", err)
	}
	return nil
}
