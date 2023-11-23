package importer

import (
	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
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

func ApplyDVCRDestinationSettings(podEnvVars *Settings, dvcrSettings *dvcr.Settings, dvcrImageName string) {
	podEnvVars.DestinationAuthSecret = dvcrSettings.AuthSecret
	podEnvVars.DestinationInsecureTLS = dvcrSettings.InsecureTLS
	podEnvVars.DestinationEndpoint = dvcrImageName
}

// ApplyHTTPSourceSettings updates importer Pod settings to use http source.
func ApplyHTTPSourceSettings(podEnvVars *Settings, http *virtv2alpha1.DataSourceHTTP) {
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

// ApplyRegistrySourceSettings updates importer Pod settings to use registry source.
func ApplyRegistrySourceSettings(podEnvVars *Settings, ctrImg *virtv2alpha1.DataSourceContainerRegistry) {
	podEnvVars.Source = cc.SourceRegistry
	podEnvVars.Endpoint = common.DockerRegistrySchemePrefix + ctrImg.Image
	// Optional auth secret.
	if secret := ctrImg.ImagePullSecret.Name; secret != "" {
		podEnvVars.AuthSecret = secret
	}
}

// ApplyDVCRSourceSettings updates importer Pod settings to use dvcr registry source.
// NOTE: no auth secret required, it will be taken from DVCR destination settings.
func ApplyDVCRSourceSettings(podEnvVars *Settings, dvcrImageName string) {
	podEnvVars.Source = cc.SourceDVCR
	podEnvVars.Endpoint = dvcrImageName
}
