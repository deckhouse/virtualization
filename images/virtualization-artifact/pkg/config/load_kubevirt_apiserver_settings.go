package config

import (
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/rest"
	"os"

	"github.com/deckhouse/virtualization-controller/pkg/common"
)

func LoadKubevirtAPIServerFromEnv() rest.KubevirtApiServerConfig {
	conf := rest.KubevirtApiServerConfig{}
	conf.Endpoint = os.Getenv(common.KubevirtAPIServerEndpointVar)
	conf.CaBundlePath = os.Getenv(common.KubevirtAPIServerCABundlePathVar)
	return conf
}
