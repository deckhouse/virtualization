/*
Copyright 2024 Flant JSC

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

package service

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/config"
)

var _ = Describe("VirtualDiskStorageClassService", func() {
	var (
		service                    *VirtualDiskStorageClassService
		storageClassSettings       config.VirtualDiskStorageClassSettings
		clusterDefaultStorageClass string
	)

	BeforeEach(func() {
		clusterDefaultStorageClass = "default-cluster-storage"
	})

	Context("when settings are empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{}
			service = NewVirtualDiskStorageClassService(storageClassSettings)
			sc := ptr.To("requested-storage-class")
			storageClass, err := service.GetStorageClass(sc, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})
	})

	Context("when settings are empty and storageClassFromSpec is empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{}
			service = NewVirtualDiskStorageClassService(storageClassSettings)

			storageClass, err := service.GetStorageClass(nil, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(BeNil())
		})
	})

	Context("when settings and clusterDefaultStorageClass are empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{}
			service = NewVirtualDiskStorageClassService(storageClassSettings)
			sc := ptr.To("requested-storage-class")
			storageClass, err := service.GetStorageClass(sc, "")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})
	})

	Context("when AllowedStorageClassNames exist, but DefaultStorageClassName is empty", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{
				AllowedStorageClassNames: []string{"allowed-storage-class"},
				DefaultStorageClassName:  "",
			}
			service = NewVirtualDiskStorageClassService(storageClassSettings)
		})

		It("returns the requested storage class if it's in the allowed list", func() {
			sc := ptr.To("allowed-storage-class")
			storageClass, err := service.GetStorageClass(sc, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})

		It("returns an error if the requested storage class is not in the allowed list", func() {
			_, err := service.GetStorageClass(ptr.To("not-allowed-storage-class"), clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})

		It("returns an error if storageClassFromSpec is empty", func() {
			_, err := service.GetStorageClass(nil, clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})

	Context("when AllowedStorageClassNames is empty, but DefaultStorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{
				AllowedStorageClassNames: []string{},
				DefaultStorageClassName:  "default-storage-class",
			}
			service = NewVirtualDiskStorageClassService(storageClassSettings)
		})

		It("returns the default storage class if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetStorageClass(nil, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(ptr.To("default-storage-class")))
		})

		It("returns the requested storage class if it matches the default storage class", func() {
			sc := ptr.To("default-storage-class")
			storageClass, err := service.GetStorageClass(sc, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})

		It("returns an error if the requested storage class does not match the default", func() {
			_, err := service.GetStorageClass(ptr.To("different-storage-class"), clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})

	Context("when both AllowedStorageClassNames and DefaultStorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{
				AllowedStorageClassNames: []string{"allowed-storage-class"},
				DefaultStorageClassName:  "default-storage-class",
			}
			service = NewVirtualDiskStorageClassService(storageClassSettings)
		})

		It("returns the default storage class if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetStorageClass(nil, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(ptr.To("default-storage-class")))
		})

		It("returns the requested storage class if it's in the allowed list", func() {
			sc := ptr.To("allowed-storage-class")
			storageClass, err := service.GetStorageClass(sc, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})

		It("returns an error if the requested storage class is not in the allowed list", func() {
			_, err := service.GetStorageClass(ptr.To("not-allowed-storage-class"), clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})
})
