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

	// ImporterPodNamePrefix provides a constant to use as a prefix for Pods created by CDI (controller only)
	ImporterPodNamePrefix = "importer"
	// ImporterPodImageNameVar is a name of variable with the image name for the importer Pods.
	ImporterPodImageNameVar = "IMPORTER_IMAGE"
	// ImporterCertDir is where the configmap containing certs will be mounted
	ImporterCertDir = "/certs"
	// ImporterCABundleDir is where the configmap containing certs from dataSource.http.caBundle field will be mounted
	ImporterCABundleDir = "/ca-bundle"
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

	// ImporterDestinationRegistryVar is a DVCR registry name.
	ImporterDestinationRegistryVar = "IMPORTER_DESTINATION_REGISTRY"
	// ImporterDestinationAuthSecretVar is a name of the Secret with auth config for DVCR.
	ImporterDestinationAuthSecretVar = "IMPORTER_DESTINATION_AUTH_SECRET"
	// ImporterDestinationInsecureTLSVar defines whether DVCR is insecure.
	ImporterDestinationInsecureTLSVar = "IMPORTER_DESTINATION_INSECURE_TLS"
	// ImporterDestinationAuthConfigDir is a mount directory for auth Secret.
	ImporterDestinationAuthConfigDir = "/ghcr-io-auth"
	// ImporterDestinationAuthConfigVar is an environment variable with auth config file for Importer Pod.
	ImporterDestinationAuthConfigVar = "IMPORTER_DESTINATION_AUTH_CONFIG"
	// ImporterDestinationAuthConfigFile is a path to auth config file in mount directory.
	ImporterDestinationAuthConfigFile = "/ghcr-io-auth/.dockerconfigjson"
	// DestinationInsecureTLSVar is an environment variable for Importer Pod that defines whether DVCR is insecure.
	DestinationInsecureTLSVar      = "DESTINATION_INSECURE_TLS"
	ImporterSHA256Sum              = "IMPORTER_SHA256SUM"
	ImporterMD5Sum                 = "IMPORTER_MD5SUM"
	ImporterAuthConfig             = "IMPORTER_AUTH_CONFIG"
	ImporterDestinationEndpoint    = "IMPORTER_DESTINATION_ENDPOINT"
	ImporterDestinationAccessKeyID = "IMPORTER_DESTINATION_ACCESS_KEY_ID"
	ImporterDestinationSecretKey   = "IMPORTER_DESTINATION_SECRET_KEY"

	// ImporterGoogleCredentialFileVar provides a constant to capture our env variable "GOOGLE_APPLICATION_CREDENTIALS"
	ImporterGoogleCredentialFileVar = "GOOGLE_APPLICATION_CREDENTIALS"
	// ImporterGoogleCredentialDir provides a constant to capture our secret mount Dir
	ImporterGoogleCredentialDir = "/google"
	// ImporterGoogleCredentialFile provides a constant to capture our credentials.json file
	ImporterGoogleCredentialFile = "/google/credentials.json"

	// ClonerSourcePodNameSuffix (controller pkg only)
	ClonerSourcePodNameSuffix = "-source-pod"
)
