package v1alpha1

type ImageStatusPhase string

const (
	PhasePending           ImageStatusPhase = "Pending"
	PhaseWaitForUserUpload ImageStatusPhase = "WaitForUserUpload"
	PhaseProvisioning      ImageStatusPhase = "Provisioning"
	PhaseReady             ImageStatusPhase = "Ready"
	PhaseFailed            ImageStatusPhase = "Failed"
	PhaseNotReady          ImageStatusPhase = "NotReady"
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
