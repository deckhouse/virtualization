package uploader

import (
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

func ApplyDVCRDestinationSettings(podEnvVars *Settings, dvcrSettings *dvcr.Settings, dvcrImageName string) {
	podEnvVars.DestinationAuthSecret = dvcrSettings.AuthSecret
	podEnvVars.DestinationInsecureTLS = dvcrSettings.InsecureTLS
	podEnvVars.DestinationEndpoint = dvcrImageName
}
