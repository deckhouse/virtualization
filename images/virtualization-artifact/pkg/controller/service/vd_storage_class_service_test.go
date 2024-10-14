package service

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
		It("returns the storageClassFromSpec if both allowed and default settings are empty", func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{}
			service = NewVirtualDiskStorageClassService(storageClassSettings, clusterDefaultStorageClass)

			storageClass, err := service.GetStorageClass("requested-storage-class")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("requested-storage-class"))
		})
	})

	Context("when settings are empty and storageClassFromSpec is empty", func() {
		It("returns the storageClassFromSpec if both allowed and default settings and clusterDefaultStorageClass are empty", func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{}
			service = NewVirtualDiskStorageClassService(storageClassSettings, clusterDefaultStorageClass)

			storageClass, err := service.GetStorageClass("")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("default-cluster-storage"))
		})
	})

	Context("when settings and clusterDefaultStorageClass are empty", func() {
		It("returns the storageClassFromSpec if both allowed and default settings and clusterDefaultStorageClass are empty", func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{}
			service = NewVirtualDiskStorageClassService(storageClassSettings, "")

			storageClass, err := service.GetStorageClass("requested-storage-class")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("requested-storage-class"))
		})
	})

	Context("when AllowedStorageClassNames exist, but DefaultStorageClassName is empty", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{
				AllowedStorageClassNames: []string{"allowed-storage-class"},
				DefaultStorageClassName:  "",
			}
			service = NewVirtualDiskStorageClassService(storageClassSettings, clusterDefaultStorageClass)
		})

		It("returns the requested storage class if it's in the allowed list", func() {
			storageClass, err := service.GetStorageClass("allowed-storage-class")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("allowed-storage-class"))
		})

		It("returns an error if the requested storage class is not in the allowed list", func() {
			_, err := service.GetStorageClass("not-allowed-storage-class")

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})

		It("returns an error if storageClassFromSpec is empty", func() {
			_, err := service.GetStorageClass("")

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})

	Context("when AllowedStorageClassNames is empty, but DefaultStorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{
				AllowedStorageClassNames: []string{},
				DefaultStorageClassName:  "default-storage-class",
			}
			service = NewVirtualDiskStorageClassService(storageClassSettings, clusterDefaultStorageClass)
		})

		It("returns the default storage class if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetStorageClass("")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("default-storage-class"))
		})

		It("returns the requested storage class if it matches the default storage class", func() {
			storageClass, err := service.GetStorageClass("default-storage-class")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("default-storage-class"))
		})

		It("returns an error if the requested storage class does not match the default", func() {
			_, err := service.GetStorageClass("different-storage-class")

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})

	Context("when both AllowedStorageClassNames and DefaultStorageClassName exist", func() {
		BeforeEach(func() {
			storageClassSettings = config.VirtualDiskStorageClassSettings{
				AllowedStorageClassNames: []string{"allowed-storage-class"},
				DefaultStorageClassName:  "default-storage-class",
			}
			service = NewVirtualDiskStorageClassService(storageClassSettings, clusterDefaultStorageClass)
		})

		It("returns the default storage class if storageClassFromSpec is empty", func() {
			storageClass, err := service.GetStorageClass("")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("default-storage-class"))
		})

		It("returns the requested storage class if it's in the allowed list", func() {
			storageClass, err := service.GetStorageClass("allowed-storage-class")

			Expect(err).To(BeNil())
			Expect(storageClass).To(Equal("allowed-storage-class"))
		})

		It("returns an error if the requested storage class is not in the allowed list", func() {
			_, err := service.GetStorageClass("not-allowed-storage-class")

			Expect(err).To(Equal(ErrStorageClassNotAvailable))
		})
	})
})
