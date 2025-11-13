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

package vdsnapshot

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(vdsnapshot *v1alpha2.VirtualDiskSnapshot)

var (
	WithName         = meta.WithName[*v1alpha2.VirtualDiskSnapshot]
	WithNamespace    = meta.WithNamespace[*v1alpha2.VirtualDiskSnapshot]
	WithGenerateName = meta.WithGenerateName[*v1alpha2.VirtualDiskSnapshot]
	WithLabel        = meta.WithLabel[*v1alpha2.VirtualDiskSnapshot]
	WithLabels       = meta.WithLabels[*v1alpha2.VirtualDiskSnapshot]
	WithAnnotation   = meta.WithAnnotation[*v1alpha2.VirtualDiskSnapshot]
	WithAnnotations  = meta.WithAnnotations[*v1alpha2.VirtualDiskSnapshot]
	WithFinalizer    = meta.WithFinalizer[*v1alpha2.VirtualDiskSnapshot]
)

func WithVirtualDiskName(virtualDiskName string) Option {
	return func(vdsnapshot *v1alpha2.VirtualDiskSnapshot) {
		vdsnapshot.Spec.VirtualDiskName = virtualDiskName
	}
}

func WithVirtualDisk(vd *v1alpha2.VirtualDisk) Option {
	return func(vdsnapshot *v1alpha2.VirtualDiskSnapshot) {
		vdsnapshot.Spec.VirtualDiskName = vd.Name
	}
}

func WithRequiredConsistency(requiredConsistency bool) Option {
	return func(vdsnapshot *v1alpha2.VirtualDiskSnapshot) {
		vdsnapshot.Spec.RequiredConsistency = requiredConsistency
	}
}
