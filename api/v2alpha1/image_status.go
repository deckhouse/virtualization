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
	ImportDuration string            `json:"importDuration"`
	DownloadSpeed  ImageStatusSpeed  `json:"downloadSpeed"`
	Size           ImageStatusSize   `json:"size"`
	CDROM          bool              `json:"cdrom"`
	Target         ImageStatusTarget `json:"target"`
	Phase          string            `json:"phase"`
	Progress       string            `json:"progress"`
	UploadCommand  string            `json:"uploadCommand"`
	FailureReason  string            `json:"failureReason"`
	FailureMessage string            `json:"failureMessage"`
}

type ImageStatusSpeed struct {
	Avg     string `json:"avg"`
	Current string `json:"current"`
}

type ImageStatusSize struct {
	Stored   string `json:"stored"`
	Unpacked string `json:"unpacked"`
}

type ImageStatusTarget struct {
	RegistryURL string `json:"registryURL"`
}
