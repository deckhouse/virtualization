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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmaccess/internal/state"
)

// KVVMIWatcher enqueues a virtual machine when what the coordinator reads from its instance changes:
// the phase, the start of its deletion, the filesystem freeze status, the guest agent connection, or a
// freeze request in flight.
type KVVMIWatcher struct{}

func NewKVVMIWatcher() *KVVMIWatcher {
	return &KVVMIWatcher{}
}

func (w *KVVMIWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{},
			&handler.TypedEnqueueRequestForObject[*virtv1.VirtualMachineInstance]{},
			predicate.TypedFuncs[*virtv1.VirtualMachineInstance]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv1.VirtualMachineInstance]) bool {
					oldVMI, newVMI := e.ObjectOld, e.ObjectNew

					return oldVMI.Status.Phase != newVMI.Status.Phase ||
						(oldVMI.DeletionTimestamp == nil) != (newVMI.DeletionTimestamp == nil) ||
						oldVMI.Status.FSFreezeStatus != newVMI.Status.FSFreezeStatus ||
						state.AgentConnected(oldVMI) != state.AgentConnected(newVMI) ||
						oldVMI.Annotations[annotations.AnnVMFilesystemRequest] != newVMI.Annotations[annotations.AnnVMFilesystemRequest]
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on internal VirtualMachineInstance: %w", err)
	}

	return nil
}
