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

	"github.com/deckhouse/virtualization-controller/pkg/config"
)

var _ = Describe("VirtualImageStorageClassService", func() {
	var (
		service                    *VirtualImageStorageClassService
		storageClassSettings       config.VirtualImageStorageClassSettings
		clusterDefaultStorageClass string
	)

	BeforeEach(func() {
		clusterDefaultStorageClass = "default-cluster-storage"
	})

	Context("when settings are empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{}
			service = NewVirtualImageStorageClassService(storageClassSettings)

			storageClass, err := service.GetStorageClass("requested-storage-class", clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("requested-storage-class"))
		})
	})

	Context("when settings are empty and storageClassFromSpec is empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{}
			service = NewVirtualImageStorageClassService(storageClassSettings)

			storageClass, err := service.GetStorageClass("", clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(""))
		})
	})

	Context("when settings and clusterDefaultStorageClass are empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{}
			service = NewVirtualImageStorageClassService(storageClassSettings)

			storageClass, err := service.GetStorageClass("requested-storage-class", "")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("requested-storage-class"))
		})
	})

	Context("when settings and clusterDefaultStorageClass are empty, but StorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{
				StorageClassName: "storage-class-name",
			}
			service = NewVirtualImageStorageClassService(storageClassSettings)
		})

		It("return the StorageClassName if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetStorageClass("", "")
			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(storageClassSettings.StorageClassName))
		})

		It("return the StorageClassName if storageClassFromSpec equal StorageClassName", func() {
			storageClass, err := service.GetStorageClass(storageClassSettings.StorageClassName, "")
			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(storageClassSettings.StorageClassName))
		})

		It("return the err if storageClassFromSpec not equal StorageClassName", func() {
			storageClass, err := service.GetStorageClass("requested-storage-class", "")
			Expect(err).To(Equal(ErrStorageClassNotAvailable))
			Expect(storageClass).To(Equal(""))
		})
	})

	Context("when AllowedStorageClassNames exist, but DefaultStorageClassName is empty", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{
				AllowedStorageClassNames: []string{"allowed-storage-class"},
				DefaultStorageClassName:  "",
			}
			service = NewVirtualImageStorageClassService(storageClassSettings)
		})

		It("returns the requested storage class if it's in the allowed list", func() {
			storageClass, err := service.GetStorageClass("allowed-storage-class", clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("allowed-storage-class"))
		})

		It("returns an error if the requested storage class is not in the allowed list", func() {
			_, err := service.GetStorageClass("not-allowed-storage-class", clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})

		It("returns an error if storageClassFromSpec is empty", func() {
			_, err := service.GetStorageClass("", clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})

	Context("when AllowedStorageClassNames is empty, but DefaultStorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{
				AllowedStorageClassNames: []string{},
				DefaultStorageClassName:  "default-storage-class",
			}
			service = NewVirtualImageStorageClassService(storageClassSettings)
		})

		It("returns the default storage class if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetStorageClass("", clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("default-storage-class"))
		})

		It("returns the requested storage class if it matches the default storage class", func() {
			storageClass, err := service.GetStorageClass("default-storage-class", clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("default-storage-class"))
		})

		It("returns an error if the requested storage class does not match the default", func() {
			_, err := service.GetStorageClass("different-storage-class", clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})

	Context("when both AllowedStorageClassNames and DefaultStorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{
				AllowedStorageClassNames: []string{"allowed-storage-class"},
				DefaultStorageClassName:  "default-storage-class",
			}
			service = NewVirtualImageStorageClassService(storageClassSettings)
		})

		It("returns the default storage class if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetStorageClass("", clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("default-storage-class"))
		})

		It("returns the requested storage class if it's in the allowed list", func() {
			storageClass, err := service.GetStorageClass("allowed-storage-class", clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("allowed-storage-class"))
		})

		It("returns an error if the requested storage class is not in the allowed list", func() {
			_, err := service.GetStorageClass("not-allowed-storage-class", clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})
})
