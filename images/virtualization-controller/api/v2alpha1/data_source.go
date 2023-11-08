package v2alpha1

// TODO: more fields from the CRD
type DataSource struct {
	Type           DataSourceType               `json:"type,omitempty"`
	HTTP           *DataSourceHTTP              `json:"http,omitempty"`
	ContainerImage *DataSourceContainerRegistry `json:"containerImage,omitempty"`
}

type DataSourceHTTP struct {
	URL      string    `json:"url"`
	CABundle []byte    `json:"caBundle"`
	Checksum *Checksum `json:"checksum,omitempty"`
}

type DataSourceContainerRegistry struct {
	Image           string          `json:"image"`
	ImagePullSecret ImagePullSecret `json:"imagePullSecret"`
	CABundle        []byte          `json:"caBundle"`
}

type ImagePullSecret struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type Checksum struct {
	MD5    string `json:"md5,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
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
