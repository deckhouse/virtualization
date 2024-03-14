package importer

import (
	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	dsutil "github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
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

func ApplyDVCRDestinationSettings(podEnvVars *Settings, dvcrSettings *dvcr.Settings, supGen *supplements.Generator, dvcrImageName string) {
	authSecret := dvcrSettings.AuthSecret
	if supplements.ShouldCopyDVCRAuthSecret(dvcrSettings, supGen) {
		authSecret = supGen.DVCRAuthSecret().Name
	}
	podEnvVars.DestinationAuthSecret = authSecret
	podEnvVars.DestinationInsecureTLS = dvcrSettings.InsecureTLS
	podEnvVars.DestinationEndpoint = dvcrImageName
}

// ApplyHTTPSourceSettings updates importer Pod settings to use http source.
func ApplyHTTPSourceSettings(podEnvVars *Settings, http *virtv2alpha1.DataSourceHTTP, supGen *supplements.Generator) {
	podEnvVars.Source = cc.SourceHTTP
	podEnvVars.Endpoint = http.URL

	if http.Checksum != nil {
		if http.Checksum.MD5 != "" {
			podEnvVars.MD5 = http.Checksum.MD5
		}

		if http.Checksum.SHA256 != "" {
			podEnvVars.SHA256 = http.Checksum.SHA256
		}
	}

	// Set ConfigMap name if caBundle is specified. ConfigMap will be created later after Pod start.
	if len(http.CABundle) > 0 {
		caBundleCM := supGen.CABundleConfigMap()
		podEnvVars.CertConfigMap = caBundleCM.Name
	}
}

// ApplyRegistrySourceSettings updates importer Pod settings to use registry source.
func ApplyRegistrySourceSettings(podEnvVars *Settings, ctrImg *virtv2alpha1.DataSourceContainerRegistry, supGen *supplements.Generator) {
	podEnvVars.Source = cc.SourceRegistry
	podEnvVars.Endpoint = common.DockerRegistrySchemePrefix + ctrImg.Image

	// Optional auth secret from imagePullSecret.
	if secretName := ctrImg.ImagePullSecret.Name; secretName != "" {
		// Copy imagePullSecret if resides in a different namespace.
		if dsutil.ShouldCopyImagePullSecret(ctrImg, supGen.Namespace) {
			imgPull := supGen.ImagePullSecret()
			podEnvVars.AuthSecret = imgPull.Name
		} else {
			podEnvVars.AuthSecret = secretName
		}
	}

	// Set ConfigMap name if caBundle is specified. ConfigMap will be created later after Pod start.
	if len(ctrImg.CABundle) > 0 {
		caBundleCM := supGen.CABundleConfigMap()
		podEnvVars.CertConfigMap = caBundleCM.Name
	}
}

// ApplyDVCRSourceSettings updates importer Pod settings to use dvcr registry source.
// NOTE: no auth secret required, it will be taken from DVCR destination settings.
func ApplyDVCRSourceSettings(podEnvVars *Settings, dvcrImageName string) {
	podEnvVars.Source = cc.SourceDVCR
	podEnvVars.Endpoint = dvcrImageName
}
