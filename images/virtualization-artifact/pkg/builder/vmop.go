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

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMOPBuilder struct {
	*ObjectMetaBuilder
	vmop *virtv2.VirtualMachineOperation
}

func NewVMOPBuilder(name, namespace string) *VMOPBuilder {
	return &VMOPBuilder{
		ObjectMetaBuilder: NewObjectMetaBuilder(name, namespace),
		vmop:              NewEmptyVMOP(name, namespace),
	}
}

func (b *VMOPBuilder) WithType(vmopType virtv2.VMOPType) *VMOPBuilder {
	b.vmop.Spec.Type = vmopType
	return b
}

func (b *VMOPBuilder) WithVirtualMachine(vm string) *VMOPBuilder {
	b.vmop.Spec.VirtualMachine = vm
	return b
}

func (b *VMOPBuilder) WithForce(force bool) *VMOPBuilder {
	b.vmop.Spec.Force = force
	return b
}

func (b *VMOPBuilder) WithStatusPhase(phase virtv2.VMOPPhase) *VMOPBuilder {
	b.vmop.Status.Phase = phase
	return b
}

func (b *VMOPBuilder) Complete() *virtv2.VirtualMachineOperation {
	vmop := b.vmop.DeepCopy()
	vmop.ObjectMeta = b.ObjectMetaBuilder.Complete()
	return vmop
}

func NewEmptyVMOP(name, namespace string) *virtv2.VirtualMachineOperation {
	return &virtv2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv2.SchemeGroupVersion.String(),
			Kind:       virtv2.VirtualMachineOperationKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}
