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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

// MigrationReceiverWatcher wakes the VirtualMachine that a request to receive a migration from a peer
// cluster names: the request has the name of the VM.
type MigrationReceiverWatcher struct{}

func NewMigrationReceiverWatcher() *MigrationReceiverWatcher {
	return &MigrationReceiverWatcher{}
}

func (w *MigrationReceiverWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{},
			&handler.TypedEnqueueRequestForObject[*intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver]{},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on InternalVirtualizationVirtualMachineReceiver: %w", err)
	}
	return nil
}
