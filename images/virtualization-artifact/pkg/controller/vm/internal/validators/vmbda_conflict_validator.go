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

package validators

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMBDAConflictValidator struct {
	client client.Client
}

func NewVMBDAConflictValidator(client client.Client) *VMBDAConflictValidator {
	return &VMBDAConflictValidator{client: client}
}

func (v *VMBDAConflictValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.checkConflicts(ctx, vm, vm.Spec.BlockDeviceRefs)
}

func (v *VMBDAConflictValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	added := findAdded(oldVM.Spec.BlockDeviceRefs, newVM.Spec.BlockDeviceRefs)
	return nil, v.checkConflicts(ctx, newVM, added)
}

func (v *VMBDAConflictValidator) checkConflicts(ctx context.Context, vm *v1alpha2.VirtualMachine, refs []v1alpha2.BlockDeviceSpecRef) error {
	if len(refs) == 0 {
		return nil
	}

	var vmbdaList v1alpha2.VirtualMachineBlockDeviceAttachmentList
	if err := v.client.List(ctx, &vmbdaList,
		client.InNamespace(vm.Namespace),
		client.MatchingFields{indexer.IndexFieldVMBDAByVM: vm.Name},
	); err != nil {
		return err
	}

	for _, vmbda := range vmbdaList.Items {
		if vmbda.Status.Phase == v1alpha2.BlockDeviceAttachmentPhaseTerminating {
			continue
		}
		ref := vmbda.Spec.BlockDeviceRef
		for _, bd := range refs {
			if string(bd.Kind) == string(ref.Kind) && bd.Name == ref.Name {
				return fmt.Errorf(
					"block device %s %q is already attached to the virtual machine via VirtualMachineBlockDeviceAttachment %q",
					bd.Kind, bd.Name, vmbda.Name,
				)
			}
		}
	}
	return nil
}

func findAdded(old, new []v1alpha2.BlockDeviceSpecRef) []v1alpha2.BlockDeviceSpecRef {
	existing := make(map[blockDeviceKey]struct{}, len(old))
	for _, bd := range old {
		existing[blockDeviceKey{Kind: bd.Kind, Name: bd.Name}] = struct{}{}
	}
	var added []v1alpha2.BlockDeviceSpecRef
	for _, bd := range new {
		if _, ok := existing[blockDeviceKey{Kind: bd.Kind, Name: bd.Name}]; !ok {
			added = append(added, bd)
		}
	}
	return added
}
