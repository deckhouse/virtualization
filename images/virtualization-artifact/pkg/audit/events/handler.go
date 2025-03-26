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

package events

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type cache interface {
	Get(key string) (any, bool)
}

type eventLogger interface {
	IsMatched(event *audit.Event) bool
	Fill(event *audit.Event) error
	ShouldLog() bool
	Log() error
}

type logMessage struct {
	Message string `json:"message"`
}

var eventLoggers = []func(NewEventHandlerOptions) eventLogger{
	NewForbid,
	NewVMManage,
	NewVMControl,
	NewVMOPControl,
	NewVMAccess,
	NewModuleComponentControl,
	NewModuleControl,
	NewIntegrityCheckVM,
}

type NewEventHandlerOptions struct {
	Ctx          context.Context
	InformerList informerList
	Client       *kubernetes.Clientset
	TTLCache     ttlCache
}

func NewEventHandler(ctx context.Context, client *kubernetes.Clientset, informerList informerList, cache cache) func([]byte) error {
	return func(line []byte) error {
		var message logMessage
		if err := json.Unmarshal(line, &message); err != nil {
			return fmt.Errorf("error parsing JSON: %w", err)
		}

		var event audit.Event
		if err := json.Unmarshal([]byte(message.Message), &event); err != nil {
			return fmt.Errorf("Error parsing JSON: %w", err)
		}

		for _, newEventLogger := range eventLoggers {
			eventLogger := newEventLogger(NewEventHandlerOptions{Ctx: ctx, Client: client, InformerList: informerList, TTLCache: cache})
			if eventLogger.IsMatched(&event) {
				if err := eventLogger.Log(&event); err != nil {
					log.Debug("fail to log event: %w", err)
				}
				break
			}
		}

		return nil
	}
}
