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

package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events/forbid"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events/integrity"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events/module"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events/vm"
)

type NewEventHandlerOptions struct {
	Ctx          context.Context
	Event        *audit.Event
	InformerList events.InformerList
	Client       *kubernetes.Clientset
	TTLCache     events.TTLCache
}

func (o NewEventHandlerOptions) GetCtx() context.Context {
	return o.Ctx
}

func (o NewEventHandlerOptions) GetEvent() *audit.Event {
	return o.Event
}

func (o NewEventHandlerOptions) GetInformerList() events.InformerList {
	return o.InformerList
}

func (o NewEventHandlerOptions) GetClient() *kubernetes.Clientset {
	return o.Client
}

func (o NewEventHandlerOptions) GetTTLCache() events.TTLCache {
	return o.TTLCache
}

type logMessage struct {
	Message string `json:"message"`
}

type NewEventLogger func(events.EventLoggerOptions) events.EventLogger

func NewEventHandler(
	ctx context.Context,
	client *kubernetes.Clientset,
	informerList events.InformerList,
	cache events.TTLCache,
) func([]byte) error {
	eL := []NewEventLogger{
		forbid.NewForbid,
		vm.NewVMManage,
		vm.NewVMControl,
		vm.NewVMOPControl,
		vm.NewVMAccess,
		module.NewModuleComponentControl,
		module.NewModuleControl,
		integrity.NewIntegrityCheckVM,
	}

	return func(line []byte) error {
		var message logMessage
		if err := json.Unmarshal(line, &message); err != nil {
			return fmt.Errorf("error parsing JSON: %w", err)
		}

		var event audit.Event
		if err := json.Unmarshal([]byte(message.Message), &event); err != nil {
			return fmt.Errorf("Error parsing JSON: %w", err)
		}

		for _, newEventLogger := range eL {
			eventLogger := newEventLogger(NewEventHandlerOptions{
				Ctx:          ctx,
				Client:       client,
				InformerList: informerList,
				TTLCache:     cache,
				Event:        &event,
			})
			if !eventLogger.IsMatched() {
				continue
			}

			if err := eventLogger.Fill(); err != nil {
				log.Debug("fail to fill event: %w", err)
			}

			if !eventLogger.ShouldLog() {
				break
			}

			if err := eventLogger.Log(); err != nil {
				log.Debug("fail to log event: %w", err)
			}

			break
		}

		return nil
	}
}
