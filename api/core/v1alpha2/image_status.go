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
	ImageTerminating       ImagePhase = "Terminating"
)

type ImageStatus struct {
	DownloadSpeed ImageStatusSpeed `json:"downloadSpeed"`
	Size          ImageStatusSize  `json:"size"`
	Format        string           `json:"format,omitempty"`
	// FIXME: create ClusterImageStatus without Capacity and PersistentVolumeClaim.
	Capacity      string            `json:"capacity,omitempty"`
	CDROM         bool              `json:"cdrom,omitempty"`
	Target        ImageStatusTarget `json:"target"`
	Phase         ImagePhase        `json:"phase,omitempty"`
	Progress      string            `json:"progress,omitempty"`
	UploadCommand string            `json:"uploadCommand,omitempty"`
	// TODO remove.
	FailureReason  string `json:"failureReason,omitempty"`
	FailureMessage string `json:"failureMessage,omitempty"`
}

type StatusSpeed struct {
	Avg          string `json:"avg,omitempty"`
	AvgBytes     string `json:"avgBytes,omitempty"`
	Current      string `json:"current,omitempty"`
	CurrentBytes string `json:"currentBytes,omitempty"`
}

type ImageStatusSpeed struct {
	Avg          string `json:"avg,omitempty"`
	AvgBytes     string `json:"avgBytes,omitempty"`
	Current      string `json:"current,omitempty"`
	CurrentBytes string `json:"currentBytes,omitempty"`
}

type ImageStatusSize struct {
	Stored        string `json:"stored,omitempty"`
	StoredBytes   string `json:"storedBytes,omitempty"`
	Unpacked      string `json:"unpacked,omitempty"`
	UnpackedBytes string `json:"unpackedBytes,omitempty"`
}

type ImageStatusTarget struct {
	RegistryURL string `json:"registryURL,omitempty"`
	// FIXME: create ClusterImageStatus without Capacity and PersistentVolumeClaim
	PersistentVolumeClaim string `json:"persistentVolumeClaimName,omitempty"`
}
