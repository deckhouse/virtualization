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

import "k8s.io/apimachinery/pkg/types"

type ImagePhase string

const (
	ImagePending           ImagePhase = "Pending"
	ImageWaitForUserUpload ImagePhase = "WaitForUserUpload"
	ImageProvisioning      ImagePhase = "Provisioning"
	ImageReady             ImagePhase = "Ready"
	ImageFailed            ImagePhase = "Failed"
	ImageTerminating       ImagePhase = "Terminating"
	ImageLost              ImagePhase = "PVCLost"
)

type ImageStatus struct {
	DownloadSpeed *StatusSpeed      `json:"downloadSpeed"`
	Size          ImageStatusSize   `json:"size"`
	Format        string            `json:"format,omitempty"`
	CDROM         bool              `json:"cdrom"`
	Target        ImageStatusTarget `json:"target"`
	Phase         ImagePhase        `json:"phase,omitempty"`
	Progress      string            `json:"progress,omitempty"`
	SourceUID     *types.UID        `json:"sourceUID,omitempty"`
	// Deprecated: use ImageUploadURLs instead.
	UploadCommand   string           `json:"uploadCommand,omitempty"`
	ImageUploadURLs *ImageUploadURLs `json:"imageUploadURLs,omitempty"`
}

type ImageUploadURLs struct {
	External  string `json:"external,omitempty"`
	InCluster string `json:"inCluster,omitempty"`
}

type StatusSpeed struct {
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
