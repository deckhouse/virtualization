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
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineWatcher struct {
	client client.Client
	logger *log.Logger
}

func NewVirtualMachineWatcher(client client.Client) *VirtualMachineWatcher {
	return &VirtualMachineWatcher{
		client: client,
		logger: log.Default().With("watcher", strings.ToLower(v1alpha2.VirtualMachineKind)),
	}
}

func (w VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(w.enqueueRequests),
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					oldVM := e.ObjectOld
					newVM := e.ObjectNew

					if oldVM.Spec.VirtualMachineIPAddress != newVM.Spec.VirtualMachineIPAddress ||
						oldVM.Status.VirtualMachineIPAddress != newVM.Status.VirtualMachineIPAddress {
						return true
					}

					if network.HasMainNetworkStatus(oldVM.Status.Networks) != network.HasMainNetworkStatus(newVM.Status.Networks) {
						return true
					}

					if network.HasMainNetworkSpec(oldVM.Spec.Networks) != network.HasMainNetworkSpec(newVM.Spec.Networks) {
						return true
					}

					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}

func (w VirtualMachineWatcher) enqueueRequests(ctx context.Context, vm *v1alpha2.VirtualMachine) []reconcile.Request {
	var requests []reconcile.Request

	vmipNames := make(map[string]struct{})

	if vm.Spec.VirtualMachineIPAddress != "" {
		vmipNames[vm.Spec.VirtualMachineIPAddress] = struct{}{}
	}

	if vm.Status.VirtualMachineIPAddress != "" {
		vmipNames[vm.Status.VirtualMachineIPAddress] = struct{}{}
	}

	vmips := &v1alpha2.VirtualMachineIPAddressList{}
	err := w.client.List(ctx, vmips, client.InNamespace(vm.Namespace), &client.MatchingFields{
		indexer.IndexFieldVMIPByVM: vm.Name,
	})
	if err != nil {
		w.logger.Error(fmt.Sprintf("failed to list vmips: %s", err))
		return nil
	}

	for _, vmip := range vmips.Items {
		vmipNames[vmip.Name] = struct{}{}
	}

	for vmipName := range vmipNames {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: vm.Namespace,
			Name:      vmipName,
		}})
	}

	return requests
}
