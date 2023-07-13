package v2alpha1

// TODO: more fields from the CRD
type DataSource struct {
	Type DataSourceType  `json:"type"`
	HTTP *DataSourceHTTP `json:"http,omitempty"`
}

type DataSourceHTTP struct {
	URL      string `json:"url"`
	CABundle []byte `json:"caBundle"`
}

type DataSourceType string

const (
	DataSourceTypeHTTP                       DataSourceType = "HTTP"
	DataSourceTypeContainerImage             DataSourceType = "ContainerImage"
	DataSourceTypeVirtualMachineImage        DataSourceType = "VirtualMachineImage"
	DataSourceTypeClusterVirtualMachineImage DataSourceType = "ClusterVirtualMachineImage"
	DataSourceTypeVirtualMachineDisk         DataSourceType = "VirtualMachineDisk"
	DataSourceTypeVirtualMachineDiskSnapshot DataSourceType = "VirtualMachineDiskSnapshot"
	DataSourceTypePersistentVolumeClaim      DataSourceType = "PersistentVolumeClaim"
	DataSourceTypeUpload                     DataSourceType = "Upload"
)
