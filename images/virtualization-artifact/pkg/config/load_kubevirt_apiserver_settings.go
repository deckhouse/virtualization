package config

import (
	"os"

	"github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/common"
)

func LoadKubevirtAPIServerFromEnv() rest.KubevirtApiServerConfig {
	conf := rest.KubevirtApiServerConfig{}
	conf.Endpoint = os.Getenv(common.KubevirtAPIServerEndpointVar)
	conf.CaBundlePath = os.Getenv(common.KubevirtAPIServerCABundlePathVar)
	conf.ServiceAccount.Name = os.Getenv(common.VirtualizationApiAuthServiceAccountNameVar)
	conf.ServiceAccount.Namespace = os.Getenv(common.VirtualizationApiAuthServiceAccountNamespaceVar)
	return conf
}
