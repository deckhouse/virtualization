/*
Copyright 2026 Flant JSC

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

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

// KVVMWatcher enqueues a virtual machine when its internal virtual machine appears, goes away, or its
// access lock changes.
type KVVMWatcher struct{}

func NewKVVMWatcher() *KVVMWatcher {
	return &KVVMWatcher{}
}

func (w *KVVMWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachine{},
			&handler.TypedEnqueueRequestForObject[*virtv1.VirtualMachine]{},
			predicate.TypedFuncs[*virtv1.VirtualMachine]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv1.VirtualMachine]) bool {
					return e.ObjectOld.GetAnnotations()[annotations.AnnVMAccessLock] != e.ObjectNew.GetAnnotations()[annotations.AnnVMAccessLock]
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on internal VirtualMachine: %w", err)
	}

	return nil
}
