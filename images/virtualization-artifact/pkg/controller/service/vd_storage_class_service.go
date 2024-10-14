package service

import "github.com/deckhouse/virtualization-controller/pkg/config"

type VirtualDiskStorageClassService struct {
	storageClassSettings config.VirtualDiskStorageClassSettings
}

func NewVirtualDiskStorageClassService(settings config.VirtualDiskStorageClassSettings) *VirtualDiskStorageClassService {
	return &VirtualDiskStorageClassService{
		storageClassSettings: settings,
	}
}

func (svc *VirtualDiskStorageClassService) GetStorageClass(storageClassFromSpec string) string {
	// TODO:

	return storageClassFromSpec
}
