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

	"github.com/deckhouse/deckhouse/pkg/log"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineWatcher struct {
	client client.Client
	logger *log.Logger
}

func NewVirtualMachineWatcher(client client.Client) *VirtualMachineWatcher {
	return &VirtualMachineWatcher{
		client: client,
		logger: log.Default().With("watcher", strings.ToLower(virtv2.VirtualMachineKind)),
	}
}

func (w VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	return ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(w.enqueueRequests),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVM := e.ObjectOld.(*virtv2.VirtualMachine)
				newVM := e.ObjectNew.(*virtv2.VirtualMachine)
				return oldVM.Spec.VirtualMachineIPAddress != newVM.Spec.VirtualMachineIPAddress ||
					oldVM.Status.VirtualMachineIPAddress != newVM.Status.VirtualMachineIPAddress
			},
		},
	)
}

func (w VirtualMachineWatcher) enqueueRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return nil
	}

	var requests []reconcile.Request

	vmipNames := make(map[string]struct{})

	if vm.Spec.VirtualMachineIPAddress != "" {
		vmipNames[vm.Spec.VirtualMachineIPAddress] = struct{}{}
	}

	if vm.Status.VirtualMachineIPAddress != "" {
		vmipNames[vm.Status.VirtualMachineIPAddress] = struct{}{}
	}

	vmips := &virtv2.VirtualMachineIPAddressList{}
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
