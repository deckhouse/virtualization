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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func New(options ...Option) *virtv2.VirtualMachineOperation {
	vmop := NewEmpty("", "")
	ApplyOptions(vmop, options...)
	return vmop
}

func ApplyOptions(vmop *virtv2.VirtualMachineOperation, opts ...Option) {
	if vmop == nil {
		return
	}
	for _, opt := range opts {
		opt(vmop)
	}
}

func NewEmpty(name, namespace string) *virtv2.VirtualMachineOperation {
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
