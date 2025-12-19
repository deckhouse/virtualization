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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func New(options ...Option) *v1alpha2.VirtualMachineSnapshot {
	vmsnapshot := NewEmpty("", "")
	ApplyOptions(vmsnapshot, options)
	return vmsnapshot
}

func ApplyOptions(vmsnapshot *v1alpha2.VirtualMachineSnapshot, opts []Option) {
	if vmsnapshot == nil {
		return
	}
	for _, opt := range opts {
		opt(vmsnapshot)
	}
}

func NewEmpty(name, namespace string) *v1alpha2.VirtualMachineSnapshot {
	return &v1alpha2.VirtualMachineSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineSnapshotKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{
			RequiredConsistency: true,                         // Default value from kubebuilder annotation
			KeepIPAddress:       v1alpha2.KeepIPAddressAlways, // Default value from kubebuilder annotation
		},
	}
}
