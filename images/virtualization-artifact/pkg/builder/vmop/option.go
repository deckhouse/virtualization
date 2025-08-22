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

package vmop

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(vmop *virtv2.VirtualMachineOperation)

var (
	WithName         = meta.WithName[*virtv2.VirtualMachineOperation]
	WithGenerateName = meta.WithGenerateName[*virtv2.VirtualMachineOperation]
	WithNamespace    = meta.WithNamespace[*virtv2.VirtualMachineOperation]
	WithLabel        = meta.WithLabel[*virtv2.VirtualMachineOperation]
	WithLabels       = meta.WithLabels[*virtv2.VirtualMachineOperation]
	WithAnnotation   = meta.WithAnnotation[*virtv2.VirtualMachineOperation]
	WithAnnotations  = meta.WithAnnotations[*virtv2.VirtualMachineOperation]
	WithFinalizer    = meta.WithFinalizer[*virtv2.VirtualMachineOperation]
)

func WithType(t virtv2.VMOPType) Option {
	return func(vmop *virtv2.VirtualMachineOperation) {
		vmop.Spec.Type = t
	}
}

func WithVirtualMachine(vm string) Option {
	return func(vmop *virtv2.VirtualMachineOperation) {
		vmop.Spec.VirtualMachine = vm
	}
}

func WithForce(force *bool) Option {
	return func(vmop *virtv2.VirtualMachineOperation) {
		vmop.Spec.Force = force
	}
}
