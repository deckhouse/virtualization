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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMMACWatcher() *VMMACWatcher {
	return &VMMACWatcher{}
}

type VMMACWatcher struct{}

func (w *VMMACWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineMACAddress{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			vmmac, ok := obj.(*virtv2.VirtualMachineMACAddress)
			if !ok {
				return nil
			}
			name := vmmac.Status.VirtualMachine
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
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVmmac := e.ObjectOld.(*virtv2.VirtualMachineMACAddress)
				newVmmac := e.ObjectNew.(*virtv2.VirtualMachineMACAddress)
				return oldVmmac.Status.Phase != newVmmac.Status.Phase ||
					oldVmmac.Status.VirtualMachine != newVmmac.Status.VirtualMachine
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineMACAddress: %w", err)
	}
	return nil
}
