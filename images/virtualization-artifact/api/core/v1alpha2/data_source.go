package v1alpha2

type DataSourceNamedRef struct {
	Name string `json:"name"`
}

type DataSourceNameNamespacedRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
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

type DataSourceClusterVirtualMachineImage struct {
	Name string `json:"name"`
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
