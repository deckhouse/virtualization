/*
Copyright 2026 Flant JSC

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

const (
	PrecreatedCVIAlpineUEFI     = "v12n-e2e-alpine-uefi"
	PrecreatedCVIAlpineBIOS     = "v12n-e2e-alpine-bios"
	PrecreatedCVIAlpineUEFIPerf = "v12n-e2e-alpine-uefi-perf"
	PrecreatedCVIAlpineBIOSPerf = "v12n-e2e-alpine-bios-perf"
	PrecreatedCVIUbuntu         = "v12n-e2e-ubuntu"
	PrecreatedCVIUbuntuISO      = "v12n-e2e-ubuntu-iso"
	PrecreatedCVIContainerImage = "v12n-e2e-container-image"
	PrecreatedCVILegacyRegistry = "v12n-e2e-legacy-registry"
	PrecreatedCVICirros         = "v12n-e2e-cirros"
	PrecreatedCVITestDataQCOW   = "v12n-e2e-testdata-qcow"
	PrecreatedCVITestDataISO    = "v12n-e2e-testdata-iso"
)

// PrecreatedClusterVirtualImages returns the suite-wide CVIs shared by e2e tests.
func PrecreatedClusterVirtualImages() []*v1alpha2.ClusterVirtualImage {
	return []*v1alpha2.ClusterVirtualImage{
		newPrecreatedHTTPCVI(PrecreatedCVIAlpineUEFI, ImageURLAlpineUEFI),
		newPrecreatedHTTPCVI(PrecreatedCVIAlpineBIOS, ImageURLAlpineBIOS),
		newPrecreatedHTTPCVI(PrecreatedCVIAlpineUEFIPerf, ImagesURLAlpineUEFIPerf),
		newPrecreatedHTTPCVI(PrecreatedCVIAlpineBIOSPerf, ImagesURLAlpineBIOSPerf),
		newPrecreatedHTTPCVI(PrecreatedCVIUbuntu, ImageURLUbuntu),
		newPrecreatedHTTPCVI(PrecreatedCVIUbuntuISO, ImageURLUbuntuISO),
		newPrecreatedContainerImageCVI(PrecreatedCVIContainerImage, ImageURLContainerImage),
		newPrecreatedContainerImageCVI(PrecreatedCVILegacyRegistry, ImageURLLegacyContainerImage),
		newPrecreatedHTTPCVI(PrecreatedCVICirros, ImageURLCirros),
		newPrecreatedHTTPCVI(PrecreatedCVITestDataQCOW, ImageTestDataQCOW),
		newPrecreatedHTTPCVI(PrecreatedCVITestDataISO, ImageTestDataISO),
	}
}

func newPrecreatedHTTPCVI(name, imageURL string) *v1alpha2.ClusterVirtualImage {
	return cvi.New(
		cvi.WithName(name),
		cvi.WithDataSourceHTTP(imageURL, nil, nil),
	)
}

func newPrecreatedContainerImageCVI(name, imageURL string) *v1alpha2.ClusterVirtualImage {
	return cvi.New(
		cvi.WithName(name),
		cvi.WithDataSourceContainerImage(imageURL, v1alpha2.ImagePullSecret{}, nil),
	)
}
