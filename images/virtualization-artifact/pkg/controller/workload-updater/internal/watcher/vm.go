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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Watch subscribes to VM resources and reconciles only those with the TypeFirmwareUpToDate condition set to False.
// While we could rely solely on the kvvmi resource's status.LauncherContainerImageVersion to determine firmware updates,
// this approach avoids duplicating logic and maintains the contract with the TypeFirmwareUpToDate condition.
func (w *VMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{},
			&handler.TypedEnqueueRequestForObject[*v1alpha2.VirtualMachine]{},
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				CreateFunc: func(e event.TypedCreateEvent[*v1alpha2.VirtualMachine]) bool {
					return predicateVirtualMachine(e.Object)
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*v1alpha2.VirtualMachine]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					return predicateVirtualMachine(e.ObjectNew)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}
	return nil
}

func predicateVirtualMachine(vm *v1alpha2.VirtualMachine) bool {
	c, _ := conditions.GetCondition(vmcondition.TypeFirmwareUpToDate, vm.Status.Conditions)
	outOfDate := c.Status == metav1.ConditionFalse

	c, _ = conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
	running := c.Status == metav1.ConditionTrue

	c, _ = conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	migrating := c.Status == metav1.ConditionTrue

	return outOfDate && running && !migrating
}
