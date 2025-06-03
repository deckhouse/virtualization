/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"os"

	"github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
)

const (
	KubevirtAPIServerEndpointVar                    = "KUBEVIRT_APISERVER_ENDPOINT"
	KubevirtAPIServerCABundlePathVar                = "KUBEVIRT_APISERVER_CABUNDLE"
	VirtualizationAPIAuthServiceAccountNameVar      = "VIRTUALIZATION_API_AUTH_SERVICE_ACCOUNT_NAME"
	VirtualizationAPIAuthServiceAccountNamespaceVar = "VIRTUALIZATION_API_AUTH_SERVICE_ACCOUNT_NAMESPACE"
)

func LoadKubevirtAPIServerFromEnv() rest.KubevirtAPIServerConfig {
	conf := rest.KubevirtAPIServerConfig{}
	conf.Endpoint = os.Getenv(KubevirtAPIServerEndpointVar)
	conf.CaBundlePath = os.Getenv(KubevirtAPIServerCABundlePathVar)
	conf.ServiceAccount.Name = os.Getenv(VirtualizationAPIAuthServiceAccountNameVar)
	conf.ServiceAccount.Namespace = os.Getenv(VirtualizationAPIAuthServiceAccountNamespaceVar)
	return conf
}
