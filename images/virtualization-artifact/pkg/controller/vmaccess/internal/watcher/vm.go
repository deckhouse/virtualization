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

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VirtualMachineWatcher enqueues a virtual machine when it appears or goes away: a claim is bound to its
// UID, and nothing else about it changes a decision.
type VirtualMachineWatcher struct{}

func NewVirtualMachineWatcher() *VirtualMachineWatcher {
	return &VirtualMachineWatcher{}
}

func (w *VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{},
			&handler.TypedEnqueueRequestForObject[*v1alpha2.VirtualMachine]{},
			predicate.TypedFuncs[*v1alpha2.VirtualMachine]{
				UpdateFunc: func(_ event.TypedUpdateEvent[*v1alpha2.VirtualMachine]) bool {
					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}

	return nil
}
