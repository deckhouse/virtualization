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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMMACWatcher() *VMMACWatcher {
	return &VMMACWatcher{}
}

type VMMACWatcher struct{}

func (w *VMMACWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachineMACAddress{},
			handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vmmac *v1alpha2.VirtualMachineMACAddress) []reconcile.Request {
				name := vmmac.Status.VirtualMachine
				if name == "" {
					for _, ownerRef := range vmmac.OwnerReferences {
						if ownerRef.Kind == v1alpha2.VirtualMachineKind && string(ownerRef.UID) == vmmac.Labels[annotations.LabelVirtualMachineUID] {
							name = ownerRef.Name
							break
						}
					}
				}

				if name == "" {
					return nil
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      name,
							Namespace: vmmac.GetNamespace(),
						},
					},
				}
			}),
			predicate.TypedFuncs[*v1alpha2.VirtualMachineMACAddress]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachineMACAddress]) bool {
					return true
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachineMACAddress]) bool {
					return true
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachineMACAddress]) bool {
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						e.ObjectOld.Status.VirtualMachine != e.ObjectNew.Status.VirtualMachine
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineMACAddress: %w", err)
	}
	return nil
}
