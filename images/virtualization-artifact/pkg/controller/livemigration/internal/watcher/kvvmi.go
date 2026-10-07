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

	"k8s.io/apimachinery/pkg/api/equality"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func NewKVVMIWatcher() *KVVMIWatcher {
	return &KVVMIWatcher{}
}

type KVVMIWatcher struct{}

func (w *KVVMIWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	// Subscribe to KVVMI status updates.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{},
			&handler.TypedEnqueueRequestForObject[*virtv1.VirtualMachineInstance]{},
			predicate.TypedFuncs[*virtv1.VirtualMachineInstance]{
				CreateFunc: func(e event.TypedCreateEvent[*virtv1.VirtualMachineInstance]) bool { return false },
				DeleteFunc: func(e event.TypedDeleteEvent[*virtv1.VirtualMachineInstance]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv1.VirtualMachineInstance]) bool {
					return !equality.Semantic.DeepEqual(e.ObjectOld.Status, e.ObjectNew.Status)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineInstance: %w", err)
	}
	return nil
}
