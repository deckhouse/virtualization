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
		UploaderIngressSettings: dvcr.UploaderIngressSettings{
			Host:               os.Getenv(common.UploaderIngressHostVar),
			TLSSecret:          os.Getenv(common.UploaderIngressTLSSecretVar),
			TLSSecretNamespace: os.Getenv(common.UploaderIngressTLSSecretNS),
			Class:              os.Getenv(common.UploaderIngressClassVar),
		},
	}

	if dvcrSettings.RegistryURL == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.DVCRRegistryURLVar)
	}
	if dvcrSettings.UploaderIngressSettings.Host == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.UploaderIngressHostVar)
	}
	if dvcrSettings.UploaderIngressSettings.TLSSecret == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.UploaderIngressTLSSecretVar)
	}
	if dvcrSettings.UploaderIngressSettings.Class == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify DVCR settings", common.UploaderIngressClassVar)
	}

	if dvcrSettings.AuthSecret != "" && dvcrSettings.AuthSecretNamespace == "" {
		dvcrSettings.AuthSecretNamespace = controllerNamespace
	}

	if dvcrSettings.CertsSecret != "" && dvcrSettings.CertsSecretNamespace == "" {
		dvcrSettings.CertsSecretNamespace = controllerNamespace
	}

	if dvcrSettings.UploaderIngressSettings.TLSSecret != "" && dvcrSettings.UploaderIngressSettings.TLSSecretNamespace == "" {
		dvcrSettings.UploaderIngressSettings.TLSSecretNamespace = controllerNamespace
	}

	return dvcrSettings, nil
}
