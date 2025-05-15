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
	"github.com/deckhouse/module-sdk/pkg/app"
)

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            common.CONTROLLER_CERT_CN,
	TLSSecretName: "virtualization-controller-tls",
	Namespace:     common.MODULE_NAMESPACE,
	SANs: tlscertificate.DefaultSANs([]string{
		"localhost",
		"127.0.0.1",
		// virtualization
		common.CONTROLLER_CERT_CN,
		// virtualization.d8-virtualization
		fmt.Sprintf("%s.%s", common.CONTROLLER_CERT_CN, common.MODULE_NAMESPACE),
		// virtualization.d8-virtualization.svc
		fmt.Sprintf("%s.%s.svc", common.CONTROLLER_CERT_CN, common.MODULE_NAMESPACE),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.controller.cert", common.MODULE_NAME),
	CommonCAValuesPath:   fmt.Sprintf("%s.internal.rootCA", common.MODULE_NAME),
})

func main() {
	app.Run()
}
