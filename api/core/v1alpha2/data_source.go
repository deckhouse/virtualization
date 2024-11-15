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

// Fill the image with data from some external url. Supported schemas are:
//
// * http
// * https
//
// For https schema there is an option to skip TLS verification.
type DataSourceHTTP struct {
	// A checksum of the file, provided by the url, to verify if it was downloaded correctly or wasn't changed. The file should match all specified checksums.
	Checksum *Checksum `json:"checksum,omitempty"`
	// The http url with an image. The following formats are supported:
	// * qcow2
	// * vmdk
	// * vdi
	// * iso
	// * raw
	// these formats can also be compressed with the following formats:
	// * gz
	// * xz
	// +kubebuilder:example:="https://mirror.example.com/images/slackware-15.qcow.gz"
	// +kubebuilder:validation:Pattern=`^http[s]?:\/\/(?:[a-zA-Z]|[0-9]|[$-_@.&+]|[!*\(\),]|(?:%[0-9a-fA-F][0-9a-fA-F]))+$`
	URL string `json:"url"`
	// The CA chain in base64 format to verify the url.
	// +kubebuilder:example:="YWFhCg=="
	CABundle []byte `json:"caBundle,omitempty"`
}

type ImagePullSecret struct {
	// A name of the secret containing registry credentials.
	Name string `json:"name,omitempty"`
	// A namespace where imagePullSecret is located.
	Namespace string `json:"namespace,omitempty"`
}

type ImagePullSecretName struct {
	// A name of the secret containing registry credentials which must be located in the same namespace.
	Name string `json:"name,omitempty"`
}

type Checksum struct {
	// +kubebuilder:example:="f3b59bed9f91e32fac1210184fcff6f5"
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{32}$"
	// +kubebuilder:validation:MinLength:=32
	// +kubebuilder:validation:MaxLength:=32
	MD5 string `json:"md5,omitempty"`
	// +kubebuilder:example:="78be890d71dde316c412da2ce8332ba47b9ce7a29d573801d2777e01aa20b9b5"
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{64}$"
	// +kubebuilder:validation:MinLength:=64
	// +kubebuilder:validation:MaxLength:=64
	SHA256 string `json:"sha256,omitempty"`
}

// The type of an origin of the image. Options are:
//
// * `HTTP` — create an image from a file published on http/https service at a given url
// * `ContainerImage` — create the image from image stored in container registry.
// * `ObjectRef` — fill the disk from another existing resource.
// * `Upload` — fill the image with data, uploaded by user via the special interface.
//
// +kubebuilder:validation:Enum:={HTTP,ContainerImage,ObjectRef,Upload}
type DataSourceType string

const (
	DataSourceTypeHTTP           DataSourceType = "HTTP"
	DataSourceTypeContainerImage DataSourceType = "ContainerImage"
	DataSourceTypeObjectRef      DataSourceType = "ObjectRef"
	DataSourceTypeUpload         DataSourceType = "Upload"
)
