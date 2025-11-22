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

package vmsnapshot

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(vdsnapshot *v1alpha2.VirtualMachineSnapshot)

var (
	WithName         = meta.WithName[*v1alpha2.VirtualMachineSnapshot]
	WithNamespace    = meta.WithNamespace[*v1alpha2.VirtualMachineSnapshot]
	WithGenerateName = meta.WithGenerateName[*v1alpha2.VirtualMachineSnapshot]
	WithLabel        = meta.WithLabel[*v1alpha2.VirtualMachineSnapshot]
	WithLabels       = meta.WithLabels[*v1alpha2.VirtualMachineSnapshot]
	WithAnnotation   = meta.WithAnnotation[*v1alpha2.VirtualMachineSnapshot]
	WithAnnotations  = meta.WithAnnotations[*v1alpha2.VirtualMachineSnapshot]
	WithFinalizer    = meta.WithFinalizer[*v1alpha2.VirtualMachineSnapshot]
)

func WithVirtualMachineName(vmName string) Option {
	return func(vmsnapshot *v1alpha2.VirtualMachineSnapshot) {
		vmsnapshot.Spec.VirtualMachineName = vmName
	}
}

func WithKeepIPAddress(keepIP v1alpha2.KeepIPAddress) Option {
	return func(vmsnapshot *v1alpha2.VirtualMachineSnapshot) {
		vmsnapshot.Spec.KeepIPAddress = keepIP
	}
}

func WithRequiredConsistency(requiredConsistency bool) Option {
	return func(vmsnapshot *v1alpha2.VirtualMachineSnapshot) {
		vmsnapshot.Spec.RequiredConsistency = requiredConsistency
	}
}

func WithVirtualMachineSnapshotSecretName(secretName string) Option {
	return func(vmsnapshot *v1alpha2.VirtualMachineSnapshot) {
		vmsnapshot.Status.VirtualMachineSnapshotSecretName = secretName
	}
}

func WithVirtualMachineSnapshotPhase(phase v1alpha2.VirtualMachineSnapshotPhase) Option {
	return func(vmsnapshot *v1alpha2.VirtualMachineSnapshot) {
		vmsnapshot.Status.Phase = phase
	}
}
