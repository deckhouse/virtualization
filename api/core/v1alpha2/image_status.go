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
	ImageTerminating       ImagePhase = "Terminating"
	ImageLost              ImagePhase = "ImageLost"
	ImagePVCLost           ImagePhase = "PVCLost"
)

type ImageUploadURLs struct {
	// Command to upload the image using `Ingress` from outside the cluster.
	External string `json:"external,omitempty"`
	// Command to upload the image using `Service` within the cluster.
	InCluster string `json:"inCluster,omitempty"`
}

// Image download speed from an external source. Appears only during the `Provisioning` phase.
type StatusSpeed struct {
	// Average download speed.
	// +kubebuilder:example:="1 Mbps"
	Avg string `json:"avg,omitempty"`
	// Average download speed in bytes per second.
	// +kubebuilder:example:=1012345
	AvgBytes string `json:"avgBytes,omitempty"`
	// Current download speed.
	// +kubebuilder:example:="5 Mbps"
	Current string `json:"current,omitempty"`
	// Current download speed in bytes per second.
	// +kubebuilder:example:=5123456
	CurrentBytes string `json:"currentBytes,omitempty"`
}

// Discovered sizes of the image.
type ImageStatusSize struct {
	// Image size in human-readable format.
	// +kubebuilder:example:="199M"
	Stored string `json:"stored,omitempty"`
	// Image size in bytes.
	// +kubebuilder:example:=199001234
	StoredBytes string `json:"storedBytes,omitempty"`
	// Unpacked image size in human-readable format.
	// +kubebuilder:example:="1G"
	Unpacked string `json:"unpacked,omitempty"`
	// Unpacked image size in bytes.
	// +kubebuilder:example:=1000000234
	UnpackedBytes string `json:"unpackedBytes,omitempty"`
}
