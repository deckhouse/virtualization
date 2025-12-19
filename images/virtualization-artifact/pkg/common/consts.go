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

package common

const (
	// FilesystemOverheadVar provides a constant to capture our env variable "FILESYSTEM_OVERHEAD"
	FilesystemOverheadVar = "FILESYSTEM_OVERHEAD"
	// OwnerUID provides the UID of the owner entity (either PVC or DV)
	OwnerUID = "OWNER_UID"

	// BounderContainerName provides a constant to use as a name for bounder Container
	BounderContainerName = "d8v-dvcr-bounder"
	// ImporterContainerName provides a constant to use as a name for importer Container
	ImporterContainerName = "d8v-dvcr-importer"
	// UploaderContainerName provides a constant to use as a name for uploader Container
	UploaderContainerName = "d8v-dvcr-uploader"
	// UploaderPortName provides a constant to use as a port name for uploader Service
	UploaderPortName = "uploader"
	// UploaderPort provides a constant to use as a port for uploader Service
	UploaderPort = 80
	// ImporterPodImageNameVar is a name of variable with the image name for the importer Pod
	ImporterPodImageNameVar = "IMPORTER_IMAGE"
	// UploaderPodImageNameVar is a name of variable with the image name for the uploader Pod
	UploaderPodImageNameVar = "UPLOADER_IMAGE"
	// BounderPodImageNameVar is a name of variable with the image name for the bounder Pod
	BounderPodImageNameVar = "BOUNDER_IMAGE"
	// ImporterCertDir is where the configmap containing certs will be mounted
	ImporterCertDir = "/certs"
	// ImporterProxyCertDir is where the configmap containing proxy certs will be mounted
	ImporterProxyCertDir = "/proxycerts/"

	// ImporterSource provides a constant to capture our env variable "IMPORTER_SOURCE"
	ImporterSource = "IMPORTER_SOURCE"
	// ImporterContentType provides a constant to capture our env variable "IMPORTER_CONTENTTYPE"
	ImporterContentType = "IMPORTER_CONTENTTYPE"
	// ImporterEndpoint provides a constant to capture our env variable "IMPORTER_ENDPOINT"
	ImporterEndpoint = "IMPORTER_ENDPOINT"
	// ImporterAccessKeyID provides a constant to capture our env variable "IMPORTER_ACCES_KEY_ID"
	ImporterAccessKeyID = "IMPORTER_ACCESS_KEY_ID"
	// ImporterSecretKey provides a constant to capture our env variable "IMPORTER_SECRET_KEY"
	ImporterSecretKey = "IMPORTER_SECRET_KEY"
	// ImporterImageSize provides a constant to capture our env variable "IMPORTER_IMAGE_SIZE"
	ImporterImageSize = "IMPORTER_IMAGE_SIZE"
	// ImporterCertDirVar provides a constant to capture our env variable "IMPORTER_CERT_DIR"
	ImporterCertDirVar = "IMPORTER_CERT_DIR"
	// InsecureTLSVar provides a constant to capture our env variable "INSECURE_TLS"
	InsecureTLSVar = "INSECURE_TLS"
	// ImporterDiskID provides a constant to capture our env variable "IMPORTER_DISK_ID"
	ImporterDiskID = "IMPORTER_DISK_ID"
	// ImporterUUID provides a constant to capture our env variable "IMPORTER_UUID"
	ImporterUUID = "IMPORTER_UUID"
	// ImporterReadyFile provides a constant to capture our env variable "IMPORTER_READY_FILE"
	ImporterReadyFile = "IMPORTER_READY_FILE"
	// ImporterDoneFile provides a constant to capture our env variable "IMPORTER_DONE_FILE"
	ImporterDoneFile = "IMPORTER_DONE_FILE"
	// ImporterBackingFile provides a constant to capture our env variable "IMPORTER_BACKING_FILE"
	ImporterBackingFile = "IMPORTER_BACKING_FILE"
	// ImporterThumbprint provides a constant to capture our env variable "IMPORTER_THUMBPRINT"
	ImporterThumbprint = "IMPORTER_THUMBPRINT"
	// ImportProxyHTTP provides a constant to capture our env variable "http_proxy"
	ImportProxyHTTP = "http_proxy"
	// ImportProxyHTTPS provides a constant to capture our env variable "https_proxy"
	ImportProxyHTTPS = "https_proxy"
	// ImportProxyNoProxy provides a constant to capture our env variable "no_proxy"
	ImportProxyNoProxy = "no_proxy"
	// ImporterProxyCertDirVar provides a constant to capture our env variable "IMPORTER_PROXY_CERT_DIR"
	ImporterProxyCertDirVar = "IMPORTER_PROXY_CERT_DIR"
	// ImporterFilesystemVar provides a constant to capture our env variable "IMPORTER_FILESYSTEM"
	ImporterFilesystemVar = "IMPORTER_FILESYSTEM"
	// ImporterFilesystemDir provides a constant to capture our env variable "IMPORTER_FILESYSTEM_DIR"
	ImporterFilesystemDir = "/tmp/fs"
	// ImporterBlockDeviceDir provides a constant to capture our directory for block device
	ImporterBlockDeviceDir = "/dev/xvda"

	// ImporterDestinationAuthConfigDir is a mount directory for auth Secret.
	ImporterDestinationAuthConfigDir = "/dvcr-auth"
	// ImporterDestinationAuthConfigVar is an environment variable with auth config file for Importer Pod.
	ImporterDestinationAuthConfigVar = "IMPORTER_DESTINATION_AUTH_CONFIG"
	// ImporterDestinationAuthConfigFile is a path to auth config file in mount directory.
	ImporterDestinationAuthConfigFile = "/dvcr-auth/.dockerconfigjson"
	// DestinationInsecureTLSVar is an environment variable for Importer Pod that defines whether DVCR is insecure.
	DestinationInsecureTLSVar   = "DESTINATION_INSECURE_TLS"
	ImporterSHA256Sum           = "IMPORTER_SHA256SUM"
	ImporterMD5Sum              = "IMPORTER_MD5SUM"
	ImporterAuthConfigVar       = "IMPORTER_AUTH_CONFIG"
	ImporterAuthConfigDir       = "/dvcr-src-auth"
	ImporterAuthConfigFile      = "/dvcr-src-auth/.dockerconfigjson"
	ImporterDestinationEndpoint = "IMPORTER_DESTINATION_ENDPOINT"

	UploaderDestinationEndpoint       = "UPLOADER_DESTINATION_ENDPOINT"
	UploaderDestinationAuthConfigVar  = "UPLOADER_DESTINATION_AUTH_CONFIG"
	UploaderDestinationAuthConfigDir  = "/dvcr-auth"
	UploaderDestinationAuthConfigFile = "/dvcr-auth/.dockerconfigjson"

	DockerRegistrySchemePrefix = "docker://"

	VMBlockDeviceAttachedLimit = 16

	CmpLesser  = -1
	CmpEqual   = 0
	CmpGreater = 1
)
