package config

import (
	"os"

	"github.com/deckhouse/virtualization-controller/pkg/apiserver/api"
	"github.com/deckhouse/virtualization-controller/pkg/common"
)

func LoadKubevirtAPIServerFromEnv() api.KubevirtApiServerConfig {
	conf := api.KubevirtApiServerConfig{}
	conf.Endpoint = os.Getenv(common.KubevirtAPIServerEndpointVar)
	conf.CertsPath = os.Getenv(common.KubevirtAPIServerCertsPathVar)
	return conf
}
