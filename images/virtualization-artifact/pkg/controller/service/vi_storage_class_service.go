package service

import "github.com/deckhouse/virtualization-controller/pkg/config"

type VirtualImageStorageClassService struct {
	storageClassSettings config.VirtualImageStorageClassSettings
}

func NewVirtualImageStorageClassService(settings config.VirtualImageStorageClassSettings) *VirtualImageStorageClassService {
	return &VirtualImageStorageClassService{
		storageClassSettings: settings,
	}
}

func (svc *VirtualImageStorageClassService) GetStorageClass(storageClassFromSpec string) string {
	// TODO:

	return storageClassFromSpec
}
