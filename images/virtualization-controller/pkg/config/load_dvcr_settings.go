package config

import (
	"fmt"
	"os"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

func LoadDVCRSettingsFromEnvs(controllerNamespace string) (*dvcr.Settings, error) {
	dvcrSettings := &dvcr.Settings{
		AuthSecret:           os.Getenv(common.DVCRAuthSecretVar),
		AuthSecretNamespace:  os.Getenv(common.DVCRAuthSecretNSVar),
		CertsSecret:          os.Getenv(common.DVCRCertsSecretVar),
		CertsSecretNamespace: os.Getenv(common.DVCRCertsSecretNSVar),
		RegistryURL:          os.Getenv(common.DVCRRegistryURLVar),
		InsecureTLS:          os.Getenv(common.DVCRInsecureTLSVar),
	}

	if dvcrSettings.RegistryURL == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.DVCRRegistryURLVar)
	}

	if dvcrSettings.AuthSecret != "" && dvcrSettings.AuthSecretNamespace == "" {
		dvcrSettings.AuthSecretNamespace = controllerNamespace
	}

	if dvcrSettings.CertsSecret != "" && dvcrSettings.CertsSecretNamespace == "" {
		dvcrSettings.CertsSecretNamespace = controllerNamespace
	}

	return dvcrSettings, nil
}
