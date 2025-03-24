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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

func NewVMWatcher() *VMWatcher {
	return &VMWatcher{}
}

type VMWatcher struct{}

func (w *VMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{}),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return false },
			UpdateFunc: func(e event.UpdateEvent) bool {
				vm, ok := e.ObjectNew.(*v1alpha2.VirtualMachine)
				if !ok {
					return false
				}
				_, needUpdate := conditions.GetCondition(vmcondition.TypeFirmwareNeedUpdate, vm.Status.Conditions)
				_, running := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
				_, migrating := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)

				return needUpdate && running && !migrating
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}
	return nil
}
