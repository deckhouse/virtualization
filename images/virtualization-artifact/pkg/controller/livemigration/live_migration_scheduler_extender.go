/*
Copyright 2026 Flant JSC

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

package livemigration

import (
	"context"
	"encoding/json"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/livemigration/internal/extender"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

// SchedulerExtenderPath is the URL prefix of the inbound migration scheduler extender. It must
// match the KubeSchedulerWebhookConfiguration; kube-scheduler appends the verb to it.
const SchedulerExtenderPath = "/scheduler/inbound-migration"

// The scheduler sends the pod and the candidate node names only, well below this size.
const maxSchedulerExtenderRequestBytes = 4 << 20

// SetupSchedulerExtender serves the scheduler extender on the webhook server. Every replica
// answers from its own cache, so the extender does not depend on the leader.
func SetupSchedulerExtender(mgr manager.Manager, settings extender.Settings, log *log.Logger) {
	ext := extender.NewInboundMigrationExtender(mgr.GetClient(), settings, log)
	server := mgr.GetWebhookServer()
	server.Register(SchedulerExtenderPath+"/filter", schedulerExtenderHandler(ext.Filter, log))
	server.Register(SchedulerExtenderPath+"/prioritize", schedulerExtenderHandler(ext.Prioritize, log))
}

func schedulerExtenderHandler[T any](verb func(context.Context, extender.Args) T, log *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "only POST is supported", http.StatusMethodNotAllowed)
			return
		}

		var args extender.Args
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSchedulerExtenderRequestBytes)).Decode(&args); err != nil {
			http.Error(w, "decode scheduler extender args: "+err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(verb(r.Context(), args)); err != nil {
			log.Error("Failed to write the scheduler extender response", logger.SlogErr(err))
		}
	})
}
