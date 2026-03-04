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

package object

import (
	"github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewHTTPCVIUbuntu(name string, opts ...cvi.Option) *v1alpha2.ClusterVirtualImage {
	baseOpts := []cvi.Option{
		cvi.WithName(name),
		cvi.WithDataSourceHTTP(
			ImageURLUbuntu,
			nil,
			nil,
		),
	}
	baseOpts = append(baseOpts, opts...)
	return cvi.New(baseOpts...)
}

func NewGenerateHTTPCVIUbuntu(prefix string, opts ...cvi.Option) *v1alpha2.ClusterVirtualImage {
	baseOpts := []cvi.Option{
		cvi.WithGenerateName(prefix),
		cvi.WithDataSourceHTTP(
			ImageURLUbuntu,
			nil,
			nil,
		),
	}
	baseOpts = append(baseOpts, opts...)
	return cvi.New(baseOpts...)
}

func NewContainerImageCVI(name string, opts ...cvi.Option) *v1alpha2.ClusterVirtualImage {
	baseOpts := []cvi.Option{
		cvi.WithName(name),
		cvi.WithDataSourceContainerImage(ImageURLContainerImage, v1alpha2.ImagePullSecret{}, nil),
	}
	baseOpts = append(baseOpts, opts...)
	return cvi.New(baseOpts...)
}

func NewGenerateContainerImageCVI(prefix string, opts ...cvi.Option) *v1alpha2.ClusterVirtualImage {
	baseOpts := []cvi.Option{
		cvi.WithGenerateName(prefix),
		cvi.WithDataSourceContainerImage(ImageURLContainerImage, v1alpha2.ImagePullSecret{}, nil),
	}
	baseOpts = append(baseOpts, opts...)
	return cvi.New(baseOpts...)
}
