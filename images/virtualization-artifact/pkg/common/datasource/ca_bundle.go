package datasource

import virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"

type CABundle struct {
	Type           virtv2.DataSourceType
	HTTP           *virtv2.DataSourceHTTP
	ContainerImage *virtv2.DataSourceContainerRegistry
}

func NewCABundleForCVMI(ds virtv2.CVMIDataSource) *CABundle {
	return &CABundle{
		Type:           ds.Type,
		HTTP:           ds.HTTP,
		ContainerImage: ds.ContainerImage,
	}
}

func NewCABundleForVMI(ds virtv2.VMIDataSource) *CABundle {
	return &CABundle{
		Type:           ds.Type,
		HTTP:           ds.HTTP,
		ContainerImage: ds.ContainerImage,
	}
}

func NewCABundleForVMD(ds *virtv2.VMDDataSource) *CABundle {
	return &CABundle{
		Type:           ds.Type,
		HTTP:           ds.HTTP,
		ContainerImage: ds.ContainerImage,
	}
}

func (ds *CABundle) HasCABundle() bool {
	return len(ds.GetCABundle()) > 0
}

func (ds *CABundle) GetCABundle() string {
	if ds == nil {
		return ""
	}
	switch ds.Type {
	case virtv2.DataSourceTypeHTTP:
		if ds.HTTP != nil {
			return string(ds.HTTP.CABundle)
		}
	case virtv2.DataSourceTypeContainerImage:
		if ds.ContainerImage != nil {
			return string(ds.ContainerImage.CABundle)
		}
	}
	return ""
}

func (ds *CABundle) GetContainerImage() *virtv2.DataSourceContainerRegistry {
	return ds.ContainerImage
}
