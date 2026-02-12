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

package virtualmachine

import (
	"context"
	"encoding/json"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newUnsafeIterator(reader client.Reader) *iterator {
	return &iterator{
		reader: reader,
	}
}

type iterator struct {
	reader client.Reader
}

// Iter implements iteration on objects VirtualMachine and create new DTO.
// DO NOT mutate VirtualMachine!
func (l *iterator) Iter(ctx context.Context, h handler) error {
	vms := v1alpha2.VirtualMachineList{}
	if err := l.reader.List(ctx, &vms, client.UnsafeDisableDeepCopy); err != nil {
		return err
	}

	for _, vm := range vms.Items {
		m := newDataMetric(&vm)
		m.AppliedVirtualMachineClassName = appliedClassName(&vm)
		if stop := h(m); stop {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			continue
		}
	}
	return nil
}

// appliedClassName returns the VirtualMachineClass name that is actually running on the VM.
// If there are no pending restart changes, spec value is already applied.
// Otherwise, it looks for a virtualMachineClassName change in restartAwaitingChanges
// and returns its currentValue (the one still running).
func appliedClassName(vm *v1alpha2.VirtualMachine) string {
	if len(vm.Status.RestartAwaitingChanges) == 0 {
		return vm.Spec.VirtualMachineClassName
	}

	for _, raw := range vm.Status.RestartAwaitingChanges {
		var change struct {
			Path         string `json:"path"`
			CurrentValue string `json:"currentValue"`
		}
		if err := json.Unmarshal(raw.Raw, &change); err != nil {
			continue
		}
		if change.Path == "virtualMachineClassName" {
			return change.CurrentValue
		}
	}

	// No virtualMachineClassName change among pending changes — spec value is applied.
	return vm.Spec.VirtualMachineClassName
}
