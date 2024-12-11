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

package webhook

import (
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	appconfig "github.com/deckhouse/virtualization-controller/pkg/config"
)

func SetupHTTPHooks(mgr manager.Manager, serviceAccounts appconfig.ServiceAccounts) {
	saNames := serviceAccounts.ToList()

	var hooks = map[string]http.Handler{
		ProtectResourcesPath: &webhook.Admission{Handler: newProtectHook(saNames, defaultProtectGroups)},
	}

	ws := mgr.GetWebhookServer()

	for path, hook := range hooks {
		ws.Register(path, hook)
	}
}
