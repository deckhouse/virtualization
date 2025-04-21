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

func NewVMIPWatcher() *VMIPWatcher {
	return &VMIPWatcher{}
}

type VMIPWatcher struct{}

func (w *VMIPWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddress{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			vmip, ok := obj.(*virtv2.VirtualMachineIPAddress)
			if !ok {
				return nil
			}
			name := vmip.Status.VirtualMachine
			if name == "" {
				return nil
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      name,
						Namespace: vmip.GetNamespace(),
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVmip := e.ObjectOld.(*virtv2.VirtualMachineIPAddress)
				newVmip := e.ObjectNew.(*virtv2.VirtualMachineIPAddress)
				return oldVmip.Status.Phase != newVmip.Status.Phase ||
					oldVmip.Status.VirtualMachine != newVmip.Status.VirtualMachine
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineIpAddress: %w", err)
	}
	return nil
}
