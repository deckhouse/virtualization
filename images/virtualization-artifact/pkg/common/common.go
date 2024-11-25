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
	// PodNamespaceVar is a name of variable with the namespace of the Pod (e.g. Pod with virtualization-controller).
	PodNamespaceVar = "POD_NAMESPACE"

	// FilesystemOverheadVar provides a constant to capture our env variable "FILESYSTEM_OVERHEAD"
	FilesystemOverheadVar = "FILESYSTEM_OVERHEAD"
	// OwnerUID provides the UID of the owner entity (either PVC or DV)
	OwnerUID = "OWNER_UID"

	// KeyAccess provides a constant to the accessKeyId label using in controller pkg and transport_test.go
	KeyAccess = "accessKeyId"
	// KeySecret provides a constant to the secretKey label using in controller pkg and transport_test.go
	KeySecret = "secretKey"

	// ImporterContainerName provides a constant to use as a name for importer Container
	ImporterContainerName = "importer"
	// UploaderContainerName provides a constant to use as a name for uploader Container
	UploaderContainerName = "uploader"
	// UploaderPortName provides a constant to use as a port name for uploader Service
	UploaderPortName = "uploader"
	// UploaderPort provides a constant to use as a port for uploader Service
	UploaderPort = 80
	// UploaderIngressHostVar is a env variable
	UploaderIngressHostVar = "UPLOADER_INGRESS_HOST"
	// UploaderIngressTLSSecretVar is a env variable
	UploaderIngressTLSSecretVar = "UPLOADER_INGRESS_TLS_SECRET"
	// UploaderIngressClassVar is a env variable
	UploaderIngressClassVar = "UPLOADER_INGRESS_CLASS"
	// UploaderIngressTLSSecretNS is a env variable
	UploaderIngressTLSSecretNS = "UPLOADER_INGRESS_TLS_SECRET_NAMESPACE"
	// ImporterPodImageNameVar is a name of variable with the image name for the importer Pod
	ImporterPodImageNameVar = "IMPORTER_IMAGE"
	// UploaderPodImageNameVar is a name of variable with the image name for the uploader Pod
	UploaderPodImageNameVar = "UPLOADER_IMAGE"
	// ImporterCertDir is where the configmap containing certs will be mounted
	ImporterCertDir = "/certs"
	// ImporterProxyCertDir is where the configmap containing proxy certs will be mounted
	ImporterProxyCertDir = "/proxycerts/"

	// QemuSubGid is the gid used as the qemu group in fsGroup
	QemuSubGid = int64(107)

	// AppKubernetesPartOfLabel is the Kubernetes recommended part-of label
	AppKubernetesPartOfLabel = "app.kubernetes.io/part-of"
	// AppKubernetesVersionLabel is the Kubernetes recommended version label
	AppKubernetesVersionLabel = "app.kubernetes.io/version"
	// AppKubernetesManagedByLabel is the Kubernetes recommended managed-by label
	AppKubernetesManagedByLabel = "app.kubernetes.io/managed-by"
	// AppKubernetesComponentLabel is the Kubernetes recommended component label
	AppKubernetesComponentLabel = "app.kubernetes.io/component"

	// PullPolicy provides a constant to capture our env variable "PULL_POLICY" (only used by cmd/cdi-controller/controller.go)
	PullPolicy = "PULL_POLICY"
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
	// CiphersTLSVar provides a constant to capture our env variable "TLS_CIPHERS"
	CiphersTLSVar = "TLS_CIPHERS"
	// MinVersionTLSVar provides a constant to capture our env variable "TLS_MIN_VERSION"
	MinVersionTLSVar = "TLS_MIN_VERSION"
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
	// ImporterCurrentCheckpoint provides a constant to capture our env variable "IMPORTER_CURRENT_CHECKPOINT"
	ImporterCurrentCheckpoint = "IMPORTER_CURRENT_CHECKPOINT"
	// ImporterPreviousCheckpoint provides a constant to capture our env variable "IMPORTER_PREVIOUS_CHECKPOINT"
	ImporterPreviousCheckpoint = "IMPORTER_PREVIOUS_CHECKPOINT"
	// ImporterFinalCheckpoint provides a constant to capture our env variable "IMPORTER_FINAL_CHECKPOINT"
	ImporterFinalCheckpoint = "IMPORTER_FINAL_CHECKPOINT"
	// Preallocation provides a constant to capture out env variable "PREALLOCATION"
	Preallocation = "PREALLOCATION"
	// ImportProxyHTTP provides a constant to capture our env variable "http_proxy"
	ImportProxyHTTP = "http_proxy"
	// ImportProxyHTTPS provides a constant to capture our env variable "https_proxy"
	ImportProxyHTTPS = "https_proxy"
	// ImportProxyNoProxy provides a constant to capture our env variable "no_proxy"
	ImportProxyNoProxy = "no_proxy"
	// ImporterProxyCertDirVar provides a constant to capture our env variable "IMPORTER_PROXY_CERT_DIR"
	ImporterProxyCertDirVar = "IMPORTER_PROXY_CERT_DIR"
	// InstallerPartOfLabel provides a constant to capture our env variable "INSTALLER_PART_OF_LABEL"
	InstallerPartOfLabel = "INSTALLER_PART_OF_LABEL"
	// InstallerVersionLabel provides a constant to capture our env variable "INSTALLER_VERSION_LABEL"
	InstallerVersionLabel = "INSTALLER_VERSION_LABEL"
	// ImporterExtraHeader provides a constant to include extra HTTP headers, as the prefix to a format string
	ImporterExtraHeader = "IMPORTER_EXTRA_HEADER_"
	// ImporterSecretExtraHeadersDir is where the secrets containing extra HTTP headers will be mounted
	ImporterSecretExtraHeadersDir = "/extraheaders"

	// DVCRAddressVar is an env variable holds address to DVCR registry.
	DVCRRegistryURLVar = "DVCR_REGISTRY_URL"
	// DVCRAuthSecretVar is an env variable holds the name of the Secret with DVCR auth credentials.
	DVCRAuthSecretVar = "DVCR_AUTH_SECRET"
	// DVCRAuthSecretNSVar is an env variable holds the namespace for the Secret with DVCR auth credentials.
	DVCRAuthSecretNSVar = "DVCR_AUTH_SECRET_NAMESPACE"
	// DVCRCertsSecretVar is an env variable holds the name of the Secret with DVCR certificates.
	DVCRCertsSecretVar = "DVCR_CERTS_SECRET"
	// DVCRCertsSecretNSVar is an env variable holds the namespace for the Secret with DVCR certificates.
	DVCRCertsSecretNSVar = "DVCR_CERTS_SECRET_NAMESPACE"
	// DVCRInsecureTLSVar is an env variable holds the flag whether DVCR is insecure.
	DVCRInsecureTLSVar = "DVCR_INSECURE_TLS"

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
	UploaderExtraHeader               = "UPLOADER_EXTRA_HEADER_"
	UploaderDestinationAuthConfigDir  = "/dvcr-auth"
	UploaderDestinationAuthConfigFile = "/dvcr-auth/.dockerconfigjson"
	UploaderSecretExtraHeadersDir     = "/extraheaders"

	// ImporterGoogleCredentialFileVar provides a constant to capture our env variable "GOOGLE_APPLICATION_CREDENTIALS"
	ImporterGoogleCredentialFileVar = "GOOGLE_APPLICATION_CREDENTIALS"
	// ImporterGoogleCredentialDir provides a constant to capture our secret mount Dir
	ImporterGoogleCredentialDir = "/google"
	// ImporterGoogleCredentialFile provides a constant to capture our credentials.json file
	ImporterGoogleCredentialFile = "/google/credentials.json"

	// ClonerSourcePodNameSuffix (controller pkg only)
	ClonerSourcePodNameSuffix = "-source-pod"

	// VirtualMachineCIDRs is a list of CIDRs used to allocate static IP addresses for Virtual Machines.
	VirtualMachineCIDRs = "VIRTUAL_MACHINE_CIDRS"

	// VirtualMachineIPLeasesRetentionDuration is a parameter for configuring the Virtual Machine IP address lease lifetime
	VirtualMachineIPLeasesRetentionDuration = "VIRTUAL_MACHINE_IP_LEASES_RETENTION_DURATION"

	// VirtualImageStorageClass is a parameter for configuring the storage class for Virtual Image on PVC.
	VirtualImageStorageClass = "VIRTUAL_IMAGE_STORAGE_CLASS"
	// VirtualImageDefaultStorageClass specifies the default storage class for virtual images on PVC when none is specified.
	VirtualImageDefaultStorageClass = "VIRTUAL_IMAGE_DEFAULT_STORAGE_CLASS"
	// VirtualImageAllowedStorageClasses is a parameter that lists all allowed storage classes for virtual images on PVC.
	VirtualImageAllowedStorageClasses = "VIRTUAL_IMAGE_ALLOWED_STORAGE_CLASSES"
	// VirtualDiskDefaultStorageClass specifies the default storage class for virtual disks when none is specified.
	VirtualDiskDefaultStorageClass = "VIRTUAL_DISK_DEFAULT_STORAGE_CLASS"
	// VirtualDiskAllowedStorageClasses is a parameter that lists all allowed storage classes for virtual disks.
	VirtualDiskAllowedStorageClasses = "VIRTUAL_DISK_ALLOWED_STORAGE_CLASSES"

	DockerRegistrySchemePrefix = "docker://"

	KubevirtAPIServerEndpointVar     = "KUBEVIRT_APISERVER_ENDPOINT"
	KubevirtAPIServerCABundlePathVar = "KUBEVIRT_APISERVER_CABUNDLE"

	ProvisioningPodLimitsVar   = "PROVISIONING_POD_LIMITS"
	ProvisioningPodRequestsVar = "PROVISIONING_POD_REQUESTS"

	VirtualizationApiAuthServiceAccountNameVar      = "VIRTUALIZATION_API_AUTH_SERVICE_ACCOUNT_NAME"
	VirtualizationApiAuthServiceAccountNamespaceVar = "VIRTUALIZATION_API_AUTH_SERVICE_ACCOUNT_NAMESPACE"

	GcVmopTtlVar              = "GC_VMOP_TTL"
	GcVmopScheduleVar         = "GC_VMOP_SCHEDULE"
	GcVMIMigrationTtlVar      = "GC_VMI_MIGRATION_TTL"
	GcVMIMigrationScheduleVar = "GC_VMI_MIGRATION_SCHEDULE"

	VmBlockDeviceAttachedLimit = 16

	CmpLesser  = -1
	CmpEqual   = 0
	CmpGreater = 1
)
