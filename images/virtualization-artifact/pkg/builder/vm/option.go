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

package vm

import (
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(vm *v1alpha2.VirtualMachine)

var (
	WithName         = meta.WithName[*v1alpha2.VirtualMachine]
	WithGenerateName = meta.WithGenerateName[*v1alpha2.VirtualMachine]
	WithNamespace    = meta.WithNamespace[*v1alpha2.VirtualMachine]
	WithLabel        = meta.WithLabel[*v1alpha2.VirtualMachine]
	WithLabels       = meta.WithLabels[*v1alpha2.VirtualMachine]
	WithAnnotation   = meta.WithAnnotation[*v1alpha2.VirtualMachine]
	WithAnnotations  = meta.WithAnnotations[*v1alpha2.VirtualMachine]
)

func WithCPU(cores int, coreFraction *string) Option {
	return func(vm *v1alpha2.VirtualMachine) {
		vm.Spec.CPU.Cores = cores
		if coreFraction != nil {
			vm.Spec.CPU.CoreFraction = *coreFraction
		}
	}
}

func WithMemory(size resource.Quantity) Option {
	return func(vm *v1alpha2.VirtualMachine) {
		vm.Spec.Memory.Size = size
	}
}
