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
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineMACAddressLease{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	)
}

func (w VirtualMachineMACAddressLeaseWatcher) enqueueRequests(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	lease, ok := obj.(*virtv2.VirtualMachineMACAddressLease)
	if !ok {
		return nil
	}

	var opts client.ListOptions
	vmmacRef := lease.Spec.VirtualMachineMACAddressRef
	if vmmacRef != nil && vmmacRef.Namespace != "" {
		if vmmacRef.Name != "" {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Namespace: lease.Spec.VirtualMachineMACAddressRef.Namespace,
					Name:      lease.Spec.VirtualMachineMACAddressRef.Name,
				},
			}}
		}

		opts.Namespace = vmmacRef.Namespace
	}

	var vmmacs virtv2.VirtualMachineMACAddressList
	err := w.client.List(ctx, &vmmacs, &opts)
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list vmmacs: %s", err))
		return
	}

	for _, vmmac := range vmmacs.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: vmmac.Namespace,
				Name:      vmmac.Name,
			},
		})
	}

	return
}
