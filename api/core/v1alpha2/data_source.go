package v1alpha2

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
	DataSourceTypeHTTP                  DataSourceType = "HTTP"
	DataSourceTypeContainerImage        DataSourceType = "ContainerImage"
	DataSourceTypeObjectRef             DataSourceType = "ObjectRef"
	DataSourceTypeUpload                DataSourceType = "Upload"
	DataSourceTypeVirtualDiskSnapshot   DataSourceType = "VirtualDiskSnapshot"
	DataSourceTypePersistentVolumeClaim DataSourceType = "PersistentVolumeClaim"
)
