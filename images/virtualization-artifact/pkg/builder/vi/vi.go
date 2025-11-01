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

package vi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func New(options ...Option) *v1alpha2.VirtualImage {
	vi := NewEmpty("", "")
	ApplyOptions(vi, options)
	return vi
}

func ApplyOptions(vi *v1alpha2.VirtualImage, opts []Option) {
	if vi == nil {
		return
	}
	for _, opt := range opts {
		opt(vi)
	}
}

func NewEmpty(name, namespace string) *v1alpha2.VirtualImage {
	return &v1alpha2.VirtualImage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualImageKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}
