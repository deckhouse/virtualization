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

package vmsop

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(vmsop *v1alpha2.VirtualMachineSnapshotOperation)

var (
	WithName         = meta.WithName[*v1alpha2.VirtualMachineSnapshotOperation]
	WithGenerateName = meta.WithGenerateName[*v1alpha2.VirtualMachineSnapshotOperation]
	WithNamespace    = meta.WithNamespace[*v1alpha2.VirtualMachineSnapshotOperation]
	WithLabel        = meta.WithLabel[*v1alpha2.VirtualMachineSnapshotOperation]
	WithLabels       = meta.WithLabels[*v1alpha2.VirtualMachineSnapshotOperation]
	WithAnnotation   = meta.WithAnnotation[*v1alpha2.VirtualMachineSnapshotOperation]
	WithAnnotations  = meta.WithAnnotations[*v1alpha2.VirtualMachineSnapshotOperation]
	WithFinalizer    = meta.WithFinalizer[*v1alpha2.VirtualMachineSnapshotOperation]
)

func WithType(t v1alpha2.VMSOPType) Option {
	return func(vmsop *v1alpha2.VirtualMachineSnapshotOperation) {
		vmsop.Spec.Type = t
	}
}

func WithVirtualMachineSnapshot(vms *v1alpha2.VirtualMachineSnapshot) Option {
	return func(vmsop *v1alpha2.VirtualMachineSnapshotOperation) {
		vmsop.Spec.VirtualMachineSnapshotName = vms.Name
	}
}
