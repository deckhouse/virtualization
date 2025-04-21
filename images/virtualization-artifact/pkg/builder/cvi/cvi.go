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

package cvi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func New(options ...Option) *v1alpha2.ClusterVirtualImage {
	cvi := NewEmpty("")
	ApplyOptions(cvi, options)
	return cvi
}

func ApplyOptions(cvi *v1alpha2.ClusterVirtualImage, opts []Option) {
	if cvi == nil {
		return
	}
	for _, opt := range opts {
		opt(cvi)
	}
}

func NewEmpty(name string) *v1alpha2.ClusterVirtualImage {
	return &v1alpha2.ClusterVirtualImage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
