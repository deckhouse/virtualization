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

	"github.com/deckhouse/virtualization-controller/pkg/builder/meta"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Option func(cvi *v1alpha2.ClusterVirtualImage)

var (
	WithName         = meta.WithName[*v1alpha2.ClusterVirtualImage]
	WithGenerateName = meta.WithGenerateName[*v1alpha2.ClusterVirtualImage]
	WithLabel        = meta.WithLabel[*v1alpha2.ClusterVirtualImage]
	WithLabels       = meta.WithLabels[*v1alpha2.ClusterVirtualImage]
	WithAnnotation   = meta.WithAnnotation[*v1alpha2.ClusterVirtualImage]
	WithAnnotations  = meta.WithAnnotations[*v1alpha2.ClusterVirtualImage]
	WithFinalizer    = meta.WithFinalizer[*v1alpha2.ClusterVirtualImage]
)

func WithDataSourceHTTP(url string, checksum *v1alpha2.Checksum, caBundle []byte) Option {
	return func(cvi *v1alpha2.ClusterVirtualImage) {
		cvi.Spec.DataSource = v1alpha2.ClusterVirtualImageDataSource{
			Type: v1alpha2.DataSourceTypeHTTP,
			HTTP: &v1alpha2.DataSourceHTTP{
				URL:      url,
				Checksum: checksum,
				CABundle: caBundle,
			},
		}
	}
}

func WithDataSourceContainerImage(image string, imagePullSecret v1alpha2.ImagePullSecret, caBundle []byte) Option {
	return func(cvi *v1alpha2.ClusterVirtualImage) {
		cvi.Spec.DataSource = v1alpha2.ClusterVirtualImageDataSource{
			Type: v1alpha2.DataSourceTypeContainerImage,
			ContainerImage: &v1alpha2.ClusterVirtualImageContainerImage{
				Image:           image,
				ImagePullSecret: imagePullSecret,
				CABundle:        caBundle,
			},
		}
	}
}

func WithDataSourceObjectRef(kind v1alpha2.ClusterVirtualImageObjectRefKind, name, namespace string) Option {
	return func(cvi *v1alpha2.ClusterVirtualImage) {
		cvi.Spec.DataSource = v1alpha2.ClusterVirtualImageDataSource{
			Type: v1alpha2.DataSourceTypeObjectRef,
			ObjectRef: &v1alpha2.ClusterVirtualImageObjectRef{
				Kind:      kind,
				Name:      name,
				Namespace: namespace,
			},
		}
	}
}

func WithDatasource(datasource v1alpha2.ClusterVirtualImageDataSource) func(cvi *v1alpha2.ClusterVirtualImage) {
	return func(cvi *v1alpha2.ClusterVirtualImage) {
		cvi.Spec.DataSource = datasource
	}
}

func WithPhase(phase v1alpha2.ImagePhase) func(cvi *v1alpha2.ClusterVirtualImage) {
	return func(cvi *v1alpha2.ClusterVirtualImage) {
		cvi.Status.Phase = phase
	}
}

func WithCondition(condition metav1.Condition) func(cvi *v1alpha2.ClusterVirtualImage) {
	return func(cvi *v1alpha2.ClusterVirtualImage) {
		cvi.Status.Conditions = append(cvi.Status.Conditions, condition)
	}
}

func WithCDROM(cdrom bool) func(vi *v1alpha2.ClusterVirtualImage) {
	return func(vi *v1alpha2.ClusterVirtualImage) {
		vi.Status.CDROM = cdrom
	}
}
