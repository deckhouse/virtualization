package v1alpha2

type ImagePhase string

const (
	ImagePending           ImagePhase = "Pending"
	ImageWaitForUserUpload ImagePhase = "WaitForUserUpload"
	ImageProvisioning      ImagePhase = "Provisioning"
	ImageReady             ImagePhase = "Ready"
	ImageFailed            ImagePhase = "Failed"
	ImagePVCLost           ImagePhase = "PVCLost"
	ImageUnknown           ImagePhase = "Unknown"
)

type ImageStatus struct {
	ImportDuration string           `json:"importDuration,omitempty"`
	DownloadSpeed  ImageStatusSpeed `json:"downloadSpeed"`
	Size           ImageStatusSize  `json:"size"`
	Format         string           `json:"format"`
	// FIXME: create ClusterImageStatus without Capacity and PersistentVolumeClaimName
	Capacity       string            `json:"capacity,omitempty"`
	CDROM          bool              `json:"cdrom"`
	Target         ImageStatusTarget `json:"target"`
	Phase          ImagePhase        `json:"phase"`
	Progress       string            `json:"progress,omitempty"`
	UploadCommand  string            `json:"uploadCommand,omitempty"`
	FailureReason  string            `json:"failureReason"`
	FailureMessage string            `json:"failureMessage"`
}

type ImageStatusSpeed struct {
	Avg          string `json:"avg,omitempty"`
	AvgBytes     string `json:"avgBytes,omitempty"`
	Current      string `json:"current,omitempty"`
	CurrentBytes string `json:"currentBytes,omitempty"`
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
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName,omitempty"`
}
