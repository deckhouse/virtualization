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

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
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

	// Build a map of VirtualMachineClassName by namespace/name for efficient lookup.
	kvvmClassNameMap, err := l.buildKVVMClassNameMap(ctx)
	if err != nil {
		return err
	}

	for _, vm := range vms.Items {
		m := newDataMetric(&vm)
		// Get applied class name from the map.
		if className, ok := kvvmClassNameMap[vm.Namespace+"/"+vm.Name]; ok {
			m.AppliedVirtualMachineClassName = className
		}
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

func (l *iterator) buildKVVMClassNameMap(ctx context.Context) (map[string]string, error) {
	kvvms := virtv1.VirtualMachineList{}
	if err := l.reader.List(ctx, &kvvms, client.UnsafeDisableDeepCopy); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(kvvms.Items))
	for i := range kvvms.Items {
		kvvm := &kvvms.Items[i]
		key := kvvm.Namespace + "/" + kvvm.Name
		result[key] = extractAppliedClassName(kvvm)
	}
	return result, nil
}

func extractAppliedClassName(kvvm *virtv1.VirtualMachine) string {
	if kvvm == nil {
		return ""
	}
	spec, err := kvbuilder.LoadLastAppliedSpec(kvvm)
	if err != nil || spec == nil {
		return ""
	}
	return spec.VirtualMachineClassName
}
