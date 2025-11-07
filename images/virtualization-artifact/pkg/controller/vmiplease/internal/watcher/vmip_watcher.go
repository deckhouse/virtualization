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
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineIPAddressWatcher struct {
	logger *log.Logger
	client client.Client
}

func NewVirtualMachineIPAddressWatcher(client client.Client) *VirtualMachineIPAddressWatcher {
	return &VirtualMachineIPAddressWatcher{
		logger: log.Default().With("watcher", strings.ToLower(v1alpha2.VirtualMachineIPAddressKind)),
		client: client,
	}
}

func (w VirtualMachineIPAddressWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachineIPAddress{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineIPAddress]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachineIPAddress]) bool { return false },
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineIPAddress: %w", err)
	}
	return nil
}

func (w VirtualMachineIPAddressWatcher) enqueueRequests(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (requests []reconcile.Request) {
	if vmip.Status.Address != "" {
		leaseName := ip.IPToLeaseName(vmip.Status.Address)
		if leaseName != "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: leaseName},
			})
		}
	}

	var leases v1alpha2.VirtualMachineIPAddressLeaseList
	err := w.client.List(ctx, &leases, &client.MatchingFields{
		indexer.IndexFieldVMIPLeaseByVMIP: indexer.GetVMIPLeaseIndexKeyByVMIP(vmip.GetName(), vmip.GetNamespace()),
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list leases: %s", err))
		return
	}

	for _, lease := range leases.Items {
		vmipRef := lease.Spec.VirtualMachineIPAddressRef
		if vmipRef != nil && vmipRef.Name == vmip.GetName() && vmipRef.Namespace == vmip.GetNamespace() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: lease.Name},
			})
		}
	}

	return requests
}
