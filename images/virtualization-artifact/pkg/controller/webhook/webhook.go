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
