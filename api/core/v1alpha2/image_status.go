/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	DownloadSpeed ImageStatusSpeed `json:"downloadSpeed"`
	Size          ImageStatusSize  `json:"size"`
	Format        string           `json:"format"`
	// FIXME: create ClusterImageStatus without Capacity and PersistentVolumeClaim
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
	// FIXME: create ClusterImageStatus without Capacity and PersistentVolumeClaim
	PersistentVolumeClaim string `json:"persistentVolumeClaimName,omitempty"`
}
