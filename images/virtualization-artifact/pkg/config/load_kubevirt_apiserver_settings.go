package config

import (
	"os"

	"github.com/deckhouse/virtualization-controller/pkg/apiserver/storage"
	"github.com/deckhouse/virtualization-controller/pkg/common"
)

func LoadKubevirtAPIServerFromEnv() storage.KubevirtApiServerConfig {
	conf := storage.KubevirtApiServerConfig{}
	conf.Endpoint = os.Getenv(common.KubevirtAPIServerEndpointVar)
	conf.CertsPath = os.Getenv(common.KubevirtAPIServerCertsPathVar)
	return conf
}
