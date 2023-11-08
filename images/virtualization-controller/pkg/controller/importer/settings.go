package importer

import (
	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	cc "github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
)

// Settings stores all possible settings for dvcr-importer binary.
// Fields from this struct are passed via environment variables.
type Settings struct {
	Verbose                string
	Endpoint               string
	MD5                    string
	SHA256                 string
	SecretName             string
	Source                 string
	ContentType            string
	ImageSize              string
	CertConfigMap          string
	DiskID                 string
	UUID                   string
	ReadyFile              string
	DoneFile               string
	BackingFile            string
	Thumbprint             string
	FilesystemOverhead     string
	AuthSecret             string
	InsecureTLS            bool
	HTTPProxy              string
	HTTPSProxy             string
	NoProxy                string
	CertConfigMapProxy     string
	ExtraHeaders           []string
	SecretExtraHeaders     []string
	DestinationEndpoint    string
	DestinationInsecureTLS string
	DestinationAuthSecret  string
}

func UpdateDVCRSettings(podEnvVars *Settings, dvcrSettings *common.DVCRSettings, endpoint string) {
	podEnvVars.DestinationAuthSecret = dvcrSettings.AuthSecret
	podEnvVars.DestinationInsecureTLS = dvcrSettings.InsecureTLS
	podEnvVars.DestinationEndpoint = endpoint
}

func UpdateHTTPSettings(podEnvVars *Settings, http *virtv2alpha1.DataSourceHTTP) {
	podEnvVars.Endpoint = http.URL

	if http.Checksum != nil {
		if http.Checksum.MD5 != "" {
			podEnvVars.MD5 = http.Checksum.MD5
		}

		if http.Checksum.SHA256 != "" {
			podEnvVars.SHA256 = http.Checksum.SHA256
		}
	}
}

func UpdateContainerImageSettings(podEnvVars *Settings, ctrImg *virtv2alpha1.DataSourceContainerRegistry) {
	podEnvVars.Endpoint = cc.DockerRegistrySchemePrefix + ctrImg.Image
}
