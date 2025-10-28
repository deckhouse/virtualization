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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMSOPWatcher() *VMSOPWatcher {
	return &VMSOPWatcher{}
}

type VMSOPWatcher struct{}

func (w VMSOPWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachineSnapshotOperation{},
			&handler.TypedEnqueueRequestForObject[*v1alpha2.VirtualMachineSnapshotOperation]{},
			NewSnapshotPredicate(),
		),
	)
	if err != nil {
		return fmt.Errorf("error setting watch on VMSOP: %w", err)
	}
	return nil
}
