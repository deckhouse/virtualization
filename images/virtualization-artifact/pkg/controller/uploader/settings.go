package uploader

import (
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

// Settings stores all possible settings for dvcr-uploader binary.
// Fields from this struct are passed via environment variables.
type Settings struct {
	Verbose                string
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
