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

const imageBaseURL = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru"

const (
	// Precreated CVI names
	PrecreatedCVIAlpineUEFI     = "v12n-e2e-alpine-uefi"
	PrecreatedCVIAlpineBIOS     = "v12n-e2e-alpine-bios"
	PrecreatedCVIAlpineUEFIPerf = "v12n-e2e-alpine-uefi-perf"
	PrecreatedCVIAlpineBIOSPerf = "v12n-e2e-alpine-bios-perf"
	PrecreatedCVIUbuntu         = "v12n-e2e-ubuntu"
	PrecreatedCVIUbuntuISO      = "v12n-e2e-ubuntu-iso"
	PrecreatedCVIContainerImage = "v12n-e2e-container-image"
	PrecreatedCVILegacyRegistry = "v12n-e2e-legacy-registry"
	PrecreatedCVICirros         = "v12n-e2e-cirros"
	PrecreatedCVIDebian         = "v12n-e2e-debian"
	PrecreatedCVITestDataQCOW   = "v12n-e2e-testdata-qcow"
	PrecreatedCVITestDataISO    = "v12n-e2e-testdata-iso"

	// Image URLs
	ImageURLAlpineUEFI     = imageBaseURL + "/alpine/alpine-3-23-3-uefi-base.qcow2"
	ImageURLAlpineBIOS     = imageBaseURL + "/alpine/alpine-3-23-3-bios-base.qcow2"
	ImageURLAlpineUEFIPerf = imageBaseURL + "/alpine/alpine-3-21-uefi-perf.qcow2"
	ImageURLAlpineBIOSPerf = imageBaseURL + "/alpine/alpine-3-21-bios-perf.qcow2"
	ImageURLUbuntu         = imageBaseURL + "/ubuntu/ubuntu-24.04-minimal-cloudimg-amd64.qcow2"
	ImageURLUbuntuISO      = imageBaseURL + "/ubuntu/ubuntu-24.04.2-live-server-amd64.iso"
	ImageURLCirros         = imageBaseURL + "/cirros/cirros-0.5.1.qcow2"
	ImageURLDebian         = imageBaseURL + "/debian/debian-12-with-tpm2-tools-amd64-20250814-2204.qcow2"

	ImageURLContainerImage       = "cr.yandex/crpvs5j3nh1mi2tpithr/e2e/alpine/alpine-image:latest"
	ImageURLLegacyContainerImage = "cr.yandex/crpvs5j3nh1mi2tpithr/e2e/alpine/alpine-3-20:latest"

	// Test data (not bootable)
	ImageTestDataQCOW = imageBaseURL + "/test/test.qcow2"
	ImageTestDataISO  = imageBaseURL + "/test/test.iso"
)

// PrecreatedClusterVirtualImages returns the suite-wide CVIs shared by e2e tests.
func PrecreatedClusterVirtualImages() []*v1alpha2.ClusterVirtualImage {
	return []*v1alpha2.ClusterVirtualImage{
		newPrecreatedHTTPCVI(PrecreatedCVIAlpineUEFI, ImageURLAlpineUEFI),
		newPrecreatedHTTPCVI(PrecreatedCVIAlpineBIOS, ImageURLAlpineBIOS),
		newPrecreatedHTTPCVI(PrecreatedCVIAlpineUEFIPerf, ImageURLAlpineUEFIPerf),
		newPrecreatedHTTPCVI(PrecreatedCVIAlpineBIOSPerf, ImageURLAlpineBIOSPerf),
		newPrecreatedHTTPCVI(PrecreatedCVIUbuntu, ImageURLUbuntu),
		newPrecreatedHTTPCVI(PrecreatedCVIUbuntuISO, ImageURLUbuntuISO),
		newPrecreatedContainerImageCVI(PrecreatedCVIContainerImage, ImageURLContainerImage),
		newPrecreatedContainerImageCVI(PrecreatedCVILegacyRegistry, ImageURLLegacyContainerImage),
		newPrecreatedHTTPCVI(PrecreatedCVICirros, ImageURLCirros),
		newPrecreatedHTTPCVI(PrecreatedCVIDebian, ImageURLDebian),
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
