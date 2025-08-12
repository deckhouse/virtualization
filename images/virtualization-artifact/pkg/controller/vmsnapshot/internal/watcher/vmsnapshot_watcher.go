/*
Copyright 2024 Flant JSC

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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineSnapshotWatcher struct {
	client client.Client
}

func NewVirtualMachineSnapshotWatcher(client client.Client) *VirtualMachineSnapshotWatcher {
	return &VirtualMachineSnapshotWatcher{
		client: client,
	}
}

func (w VirtualMachineSnapshotWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineSnapshot{},
			&handler.TypedEnqueueRequestForObject[*virtv2.VirtualMachineSnapshot]{},
			predicate.TypedFuncs[*virtv2.VirtualMachineSnapshot]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualMachineSnapshot]) bool {
					return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineSnapshot: %w", err)
	}
	return nil
}
