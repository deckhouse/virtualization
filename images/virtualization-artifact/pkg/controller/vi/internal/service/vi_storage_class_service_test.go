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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/config"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Service")
}

var _ = Describe("VirtualImageStorageClassService", func() {
	var (
		service                    *VirtualImageStorageClassService
		storageClassSettings       config.VirtualImageStorageClassSettings
		clusterDefaultStorageClass *storagev1.StorageClass
	)

	BeforeEach(func() {
		clusterDefaultStorageClass = &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "default-cluster-storage"}}
	})

	Context("when settings are empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{}
			service = NewVirtualImageStorageClassService(nil, storageClassSettings)
			sc := ptr.To("requested-storage-class")
			storageClass, err := service.GetValidatedStorageClass(sc, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})
	})

	Context("when settings are empty and storageClassFromSpec is empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{}
			service = NewVirtualImageStorageClassService(nil, storageClassSettings)

			storageClass, err := service.GetValidatedStorageClass(nil, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(BeNil())
		})
	})

	Context("when settings and clusterDefaultStorageClass are empty", func() {
		It("returns the storageClassFromSpec", func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{}
			service = NewVirtualImageStorageClassService(nil, storageClassSettings)
			sc := ptr.To("requested-storage-class")
			storageClass, err := service.GetValidatedStorageClass(sc, nil)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})
	})

	Context("when settings and clusterDefaultStorageClass are empty, but StorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{
				StorageClassName: "storage-class-name",
			}
			service = NewVirtualImageStorageClassService(nil, storageClassSettings)
		})

		It("return the StorageClassName if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetValidatedStorageClass(nil, nil)
			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(&storageClassSettings.StorageClassName))
		})

		It("return the StorageClassName if storageClassFromSpec equal StorageClassName", func() {
			storageClass, err := service.GetValidatedStorageClass(&storageClassSettings.StorageClassName, nil)
			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(&storageClassSettings.StorageClassName))
		})

		It("return the err if storageClassFromSpec not equal StorageClassName", func() {
			sc := ptr.To("requested-storage-class")
			_, err := service.GetValidatedStorageClass(sc, nil)
			Expect(err).To(Equal(ErrStorageClassNotAllowed))
		})
	})

	Context("when AllowedStorageClassNames exist, but DefaultStorageClassName is empty", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{
				AllowedStorageClassNames: []string{"allowed-storage-class"},
				DefaultStorageClassName:  "",
			}
			service = NewVirtualImageStorageClassService(nil, storageClassSettings)
		})

		It("returns the requested storage class if it's in the allowed list", func() {
			sc := ptr.To("allowed-storage-class")
			storageClass, err := service.GetValidatedStorageClass(sc, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})

		It("returns an error if the requested storage class is not in the allowed list", func() {
			_, err := service.GetValidatedStorageClass(ptr.To("not-allowed-storage-class"), clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAllowed))
		})

		It("returns an error if storageClassFromSpec is empty", func() {
			_, err := service.GetValidatedStorageClass(nil, clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAllowed))
		})
	})

	Context("when AllowedStorageClassNames is empty, but DefaultStorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{
				AllowedStorageClassNames: []string{},
				DefaultStorageClassName:  "default-storage-class",
			}
			service = NewVirtualImageStorageClassService(nil, storageClassSettings)
		})

		It("returns the default storage class if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetValidatedStorageClass(nil, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(ptr.To("default-storage-class")))
		})

		It("returns the requested storage class if it matches the default storage class", func() {
			sc := ptr.To("default-storage-class")
			storageClass, err := service.GetValidatedStorageClass(sc, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})

		It("returns an error if the requested storage class does not match the default", func() {
			_, err := service.GetValidatedStorageClass(ptr.To("different-storage-class"), clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAllowed))
		})
	})

	Context("when both AllowedStorageClassNames and DefaultStorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualImageStorageClassSettings{
				AllowedStorageClassNames: []string{"allowed-storage-class"},
				DefaultStorageClassName:  "default-storage-class",
			}
			service = NewVirtualImageStorageClassService(nil, storageClassSettings)
		})

		It("returns the default storage class if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetValidatedStorageClass(nil, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(ptr.To("default-storage-class")))
		})

		It("returns the requested storage class if it's in the allowed list", func() {
			sc := ptr.To("allowed-storage-class")
			storageClass, err := service.GetValidatedStorageClass(sc, clusterDefaultStorageClass)

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal(sc))
		})

		It("returns an error if the requested storage class is not in the allowed list", func() {
			_, err := service.GetValidatedStorageClass(ptr.To("not-allowed-storage-class"), clusterDefaultStorageClass)

			Expect(err).To(Equal(ErrStorageClassNotAllowed))
		})
	})

	Context("StorageProfileValidation", func() {
		BeforeEach(func() {
			service = NewVirtualImageStorageClassService(nil, config.VirtualImageStorageClassSettings{})
		})
		When("a storage profile has the volume mode `Filesystem` and the access mode `ReadWriteMany`", func() {
			It("returns an error", func() {
				sp := &cdiv1.StorageProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name: "FilesystemStorageClass",
					},
					Spec: cdiv1.StorageProfileSpec{},
					Status: cdiv1.StorageProfileStatus{
						ClaimPropertySets: []cdiv1.ClaimPropertySet{
							{
								AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
								VolumeMode:  ptr.To(v1.PersistentVolumeFilesystem),
							},
						},
					},
				}
				err := service.ValidateClaimPropertySets(sp)
				Expect(err).To(HaveOccurred())
			})
		})
		When("a storage profile has the volume mode `Block` and the access mode `ReadWriteMany`", func() {
			It("does not return an error", func() {
				sp := &cdiv1.StorageProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name: "BlockStorageClass",
					},
					Spec: cdiv1.StorageProfileSpec{},
					Status: cdiv1.StorageProfileStatus{
						ClaimPropertySets: []cdiv1.ClaimPropertySet{
							{
								AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
								VolumeMode:  ptr.To(v1.PersistentVolumeFilesystem),
							},
							{
								AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
								VolumeMode:  ptr.To(v1.PersistentVolumeBlock),
							},
							{
								AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
								VolumeMode:  ptr.To(v1.PersistentVolumeBlock),
							},
						},
					},
				}
				err := service.ValidateClaimPropertySets(sp)
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("a storage profile has the volume mode `Block` and the access mode `ReadWriteOnce`", func() {
			It("returns an error", func() {
				sp := &cdiv1.StorageProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name: "BlockStorageClass",
					},
					Spec: cdiv1.StorageProfileSpec{},
					Status: cdiv1.StorageProfileStatus{
						ClaimPropertySets: []cdiv1.ClaimPropertySet{
							{
								AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
								VolumeMode:  ptr.To(v1.PersistentVolumeBlock),
							},
							{
								AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
								VolumeMode:  ptr.To(v1.PersistentVolumeFilesystem),
							},
						},
					},
				}
				err := service.ValidateClaimPropertySets(sp)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
