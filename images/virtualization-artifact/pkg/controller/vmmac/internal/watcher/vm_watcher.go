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
	"reflect"
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
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&virtv2.VirtualMachine{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vm *virtv2.VirtualMachine) []reconcile.Request {
				vmmacNames := make(map[string]struct{})

				if len(vm.Status.Networks) > 0 {
					for _, nc := range vm.Status.Networks {
						if nc.VirtualMachineMACAddressName != "" {
							vmmacNames[nc.VirtualMachineMACAddressName] = struct{}{}
						}
					}
				}

				if len(vm.Spec.Networks) > 0 {
					for _, nc := range vm.Spec.Networks {
						if nc.VirtualMachineMACAddressName != "" {
							vmmacNames[nc.VirtualMachineMACAddressName] = struct{}{}
						}
					}
				}

				vmmacs := &virtv2.VirtualMachineMACAddressList{}
				if err := w.client.List(ctx, vmmacs, client.InNamespace(vm.Namespace), &client.MatchingFields{
					indexer.IndexFieldVMMACByVM: vm.Name,
				}); err != nil {
					w.logger.Error(fmt.Sprintf("failed to list vmmacs for vm %s/%s: %s", vm.Namespace, vm.Name, err))
					return nil
				}

				for _, vmmac := range vmmacs.Items {
					vmmacNames[vmmac.Name] = struct{}{}
				}

				var requests []reconcile.Request
				for name := range vmmacNames {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vm.Namespace,
							Name:      name,
						},
					})
				}

				return requests
			}),
			predicate.TypedFuncs[*virtv2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*virtv2.VirtualMachine]) bool {
					return true
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*virtv2.VirtualMachine]) bool {
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualMachine]) bool {
					return !reflect.DeepEqual(e.ObjectOld.Status.Networks, e.ObjectNew.Status.Networks)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}
	return nil
}
