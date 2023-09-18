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
	ImportDuration string           `json:"importDuration"`
	DownloadSpeed  ImageStatusSpeed `json:"downloadSpeed"`
	Size           ImageStatusSize  `json:"size"`
	// FIXME: create ClusterImageStatus without Capacity and PersistentVolumeClaimName
	Capacity       string            `json:"capacity,omitempty"`
	CDROM          bool              `json:"cdrom"`
	Target         ImageStatusTarget `json:"target"`
	Phase          ImagePhase        `json:"phase"`
	Progress       string            `json:"progress"`
	UploadCommand  string            `json:"uploadCommand"`
	FailureReason  string            `json:"failureReason"`
	FailureMessage string            `json:"failureMessage"`
}

type ImageStatusSpeed struct {
	Avg          string `json:"avg"`
	AvgBytes     string `json:"avgBytes"`
	Current      string `json:"current"`
	CurrentBytes string `json:"currentBytes"`
}

type ImageStatusSize struct {
	Stored        string `json:"stored"`
	StoredBytes   string `json:"storedBytes"`
	Unpacked      string `json:"unpacked"`
	UnpackedBytes string `json:"unpackedBytes"`
}

type ImageStatusTarget struct {
	RegistryURL string `json:"registryURL"`
	// FIXME: create ClusterImageStatus without Capacity and PersistentVolumeClaimName
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName"`
}
