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

// Fill the image with data from an external URL. The following schemas are supported:
//
// * HTTP
// * HTTPS
//
// For HTTPS schema, there is an option to skip the TLS verification.
type DataSourceHTTP struct {
	// Checksum to verify integrity and consistency of the downloaded file. The file must match all specified checksums.
	Checksum *Checksum `json:"checksum,omitempty"`
	// URL of the file for creating an image. The following file formats are supported:
	// * qcow2
	// * vmdk
	// * vdi
	// * iso
	// * raw
	// The file can be compressed into an archive in one of the following formats:
	// * gz
	// * xz
	// +kubebuilder:example:="https://mirror.example.com/images/slackware-15.qcow.gz"
	// +kubebuilder:validation:Pattern=`^http[s]?:\/\/(?:[a-zA-Z]|[0-9]|[$-_@.&+]|[!*\(\),]|(?:%[0-9a-fA-F][0-9a-fA-F]))+$`
	URL string `json:"url"`
	// CA chain in Base64 format to verify the URL.
	// +kubebuilder:example:="YWFhCg=="
	CABundle []byte `json:"caBundle,omitempty"`
}

// Fill the image with data the user uploads through the upload interface.
type DataSourceUpload struct {
	// Checksum to verify integrity and consistency of the uploaded file. The file must match all specified checksums.
	//
	// The checksums are calculated over the bytes the client sends, before any
	// conversion of the image, so they are the checksums of the very file being
	// uploaded.
	Checksum *Checksum `json:"checksum,omitempty"`
}

type ImagePullSecret struct {
	// Name of the secret keeping container registry credentials.
	Name string `json:"name,omitempty"`
	// Namespace where `imagePullSecret` is located.
	Namespace string `json:"namespace,omitempty"`
}

type ImagePullSecretName struct {
	// Name of the secret keeping container registry credentials, which must be located in the same namespace.
	Name string `json:"name,omitempty"`
}

type Checksum struct {
	// +kubebuilder:example:="f3b59bed9f91e32fac1210184fcff6f5"
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{32}$"
	// +kubebuilder:validation:MinLength:=32
	// +kubebuilder:validation:MaxLength:=32
	MD5 string `json:"md5,omitempty"`
	// +kubebuilder:example:="0a0a9f2a6772942557ab5355d76af442f8f65e01"
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{40}$"
	// +kubebuilder:validation:MinLength:=40
	// +kubebuilder:validation:MaxLength:=40
	SHA1 string `json:"sha1,omitempty"`
	// +kubebuilder:example:="78be890d71dde316c412da2ce8332ba47b9ce7a29d573801d2777e01aa20b9b5"
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{64}$"
	// +kubebuilder:validation:MinLength:=64
	// +kubebuilder:validation:MaxLength:=64
	SHA256 string `json:"sha256,omitempty"`
	// +kubebuilder:example:="cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{128}$"
	// +kubebuilder:validation:MinLength:=128
	// +kubebuilder:validation:MaxLength:=128
	SHA512 string `json:"sha512,omitempty"`
	// Checksum according to GOST R 34.11-2012 (Streebog), 256 bits.
	// +kubebuilder:example:="3f539a213e97c802cc229d474c6aa32a825a360b2a933a949fd925208d9ce1bb"
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{64}$"
	// +kubebuilder:validation:MinLength:=64
	// +kubebuilder:validation:MaxLength:=64
	Streebog256 string `json:"streebog256,omitempty"`
	// Checksum according to GOST R 34.11-2012 (Streebog), 512 bits.
	// +kubebuilder:example:="8e945da209aa869f0455928529bcae4679e9873ab707b55315f56ceb98bef0a7362f715528356ee83cda5f2aac4c6ad2ba3a715c1bcd81cb8e9f90bf4c1c1a8a"
	// +kubebuilder:validation:Pattern="^[0-9a-fA-F]{128}$"
	// +kubebuilder:validation:MinLength:=128
	// +kubebuilder:validation:MaxLength:=128
	Streebog512 string `json:"streebog512,omitempty"`
}

// The following image sources are available for creating an image:
//
// * `HTTP`: From a file published on an HTTP/HTTPS service at a given URL.
// * `ContainerImage`: From another image stored in a container registry.
// * `ObjectRef`: From an existing resource.
// * `Upload`: From data uploaded by the user via a special interface.
//
// +kubebuilder:validation:Enum:={HTTP,ContainerImage,ObjectRef,Upload}
type DataSourceType string

const (
	DataSourceTypeHTTP           DataSourceType = "HTTP"
	DataSourceTypeContainerImage DataSourceType = "ContainerImage"
	DataSourceTypeObjectRef      DataSourceType = "ObjectRef"
	DataSourceTypeUpload         DataSourceType = "Upload"
)
