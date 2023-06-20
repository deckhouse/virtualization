package v2alpha1

type ImagePhase string

const (
	ImagePending           ImagePhase = "Pending"
	ImageWaitForUserUpload ImagePhase = "WaitForUserUpload"
	ImageProvisioning      ImagePhase = "Provisioning"
	ImageReady             ImagePhase = "Ready"
	ImageFailed            ImagePhase = "Failed"
	ImageNotReady          ImagePhase = "NotReady"
)

type ImageStatus struct {
	Size                      string                 `json:"size"`
	CDROM                     bool                   `json:"cdrom"`
	PersistentVolumeClaimName string                 `json:"persistentVolumeClaimName"`
	RegistryURL               string                 `json:"registryURL"`
	Conditions                []ImageStatusCondition `json:"conditions"`
	Phase                     string                 `json:"phase"`
	UploadCommand             string                 `json:"uploadCommand"`
	Progress                  string                 `json:"progress"`
}

type ImageStatusCondition struct {
	LastHeartbeatTime  string `json:"lastHeartbeatTime"`
	LastTransitionTime string `json:"lastTransitionTime"`
	Message            string `json:"message"`
	Reason             string `json:"reason"`
	Status             string `json:"status"`
	Type               string `json:"type"`
}
