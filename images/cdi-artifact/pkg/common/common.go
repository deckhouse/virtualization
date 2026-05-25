package common

import (
	"encoding/json"
	"fmt"
)

const (
	ImporterVolumePath = "/data"
	DiskImageName      = "disk.img"
	ImporterWritePath  = ImporterVolumePath + "/" + DiskImageName
	WriteBlockPath     = "/dev/cdi-block-volume"
	ImporterDataDir    = "/data"
	ScratchDataDir     = "/scratch"
	NbdkitLogPath      = "/tmp/nbdkit.log"

	PodTerminationMessageFile = "/dev/termination-log"

	ImporterSource        = "IMPORTER_SOURCE"
	ImporterContentType   = "IMPORTER_CONTENTTYPE"
	ImporterEndpoint      = "IMPORTER_ENDPOINT"
	ImporterAccessKeyID   = "IMPORTER_ACCESS_KEY_ID"
	ImporterSecretKey     = "IMPORTER_SECRET_KEY"
	ImporterImageSize     = "IMPORTER_IMAGE_SIZE"
	ImporterCertDirVar    = "IMPORTER_CERT_DIR"
	ImporterDoneFile      = "IMPORTER_DONE_FILE"
	ImporterProxyCertDir  = "/proxycerts/"
	InsecureTLSVar        = "INSECURE_TLS"
	CacheMode             = "CACHE_MODE"
	CacheModeTryNone      = "TRYNONE"
	FilesystemOverheadVar = "FILESYSTEM_OVERHEAD"
	OwnerUID              = "OWNER_UID"

	GenericError         = "Error"
	PreallocationApplied = "Preallocation applied"
	ScratchSpaceRequired = "scratch space required and none found"
	ImagePullFailureText = "failed to pull image"
)

// TerminationMessage contains data to be serialized and used as the termination message of the importer.
type TerminationMessage struct {
	ScratchSpaceRequired *bool             `json:"scratchSpaceRequired,omitempty"`
	PreallocationApplied *bool             `json:"preallocationApplied,omitempty"`
	DeadlinePassed       *bool             `json:"deadlinePassed,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	Message              *string           `json:"message,omitempty"`
}

func (it *TerminationMessage) String() (string, error) {
	msg, err := json.Marshal(it)
	if err != nil {
		return "", err
	}

	// Messages longer than 4096 are truncated by kubelet.
	if length := len(msg); length > 4096 {
		return "", fmt.Errorf("termination message length %d exceeds maximum length of 4096 bytes", length)
	}

	return string(msg), nil
}

// ServerInfo contains data to be serialized and used as the body of responses to the container image server info endpoint.
type ServerInfo struct {
	Env []string `json:"env,omitempty"`
}
