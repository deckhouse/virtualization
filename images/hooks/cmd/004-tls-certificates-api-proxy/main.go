/*
Copyright 2025 Flant JSC
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

package main

import (
	"fmt"
	"hooks/pkg/common"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	v1 "k8s.io/api/certificates/v1"
)

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:                   common.API_PROXY_CERT_CN,
	TLSSecretName:        "virtualization-api-proxy-tls",
	Namespace:            common.MODULE_NAMESPACE,
	SANs:                 func(input *pkg.HookInput) []string { return []string{} },
	FullValuesPathPrefix: fmt.Sprintf("%s.internal.apiserver.proxyCert", common.MODULE_NAME),
	CommonCAValuesPath:   fmt.Sprintf("%s.internal.rootCA", common.MODULE_NAME),
	Usages:               []v1.KeyUsage{v1.UsageClientAuth},
})

func main() {
	app.Run()
}
