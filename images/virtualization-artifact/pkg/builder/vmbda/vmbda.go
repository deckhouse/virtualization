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

package vmbda

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func New(options ...Option) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	vmbda := NewEmpty("", "")
	ApplyOptions(vmbda, options)
	return vmbda
}

func ApplyOptions(vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment, opts []Option) {
	if vmbda == nil {
		return
	}
	for _, opt := range opts {
		opt(vmbda)
	}
}

func NewEmpty(name, namespace string) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return &v1alpha2.VirtualMachineBlockDeviceAttachment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineBlockDeviceAttachmentKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}
