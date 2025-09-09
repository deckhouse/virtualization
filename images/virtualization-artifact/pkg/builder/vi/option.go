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

	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(vi *v1alpha2.VirtualImage)

var (
	WithName        = meta.WithName[*v1alpha2.VirtualImage]
	WithNamespace   = meta.WithNamespace[*v1alpha2.VirtualImage]
	WithLabel       = meta.WithLabel[*v1alpha2.VirtualImage]
	WithLabels      = meta.WithLabels[*v1alpha2.VirtualImage]
	WithAnnotation  = meta.WithAnnotation[*v1alpha2.VirtualImage]
	WithAnnotations = meta.WithAnnotations[*v1alpha2.VirtualImage]
)

func WithPhase(phase v1alpha2.ImagePhase) func(vi *v1alpha2.VirtualImage) {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Status.Phase = phase
	}
}

func WithCDROM(cdrom bool) func(vi *v1alpha2.VirtualImage) {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Status.CDROM = cdrom
	}
}

func WithDatasource(datasource v1alpha2.VirtualImageDataSource) func(vi *v1alpha2.VirtualImage) {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Spec.DataSource = datasource
	}
}

func WithCondition(condition metav1.Condition) func(vi *v1alpha2.VirtualImage) {
	return func(vi *v1alpha2.VirtualImage) {
		vi.Status.Conditions = append(vi.Status.Conditions, condition)
	}
}
