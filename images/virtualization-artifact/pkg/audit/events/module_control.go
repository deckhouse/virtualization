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

package events

import (
	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type NewModuleControlOptions struct {
	NodeInformer         indexer
	ModuleInformer       indexer
	ModuleConfigInformer indexer
}

func NewModuleControl(options NewModuleControlOptions) *ModuleControl {
	return &ModuleControl{
		nodeInformer:         options.NodeInformer,
		moduleInformer:       options.ModuleInformer,
		moduleConfigInformer: options.ModuleConfigInformer,
	}
}

type ModuleControl struct {
	nodeInformer         indexer
	moduleInformer       indexer
	moduleConfigInformer indexer
}

func (m *ModuleControl) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if event.Verb == "get" || event.Verb == "list" {
		return false
	}

	if event.ObjectRef.Resource == "moduleconfigs" {
		return true
	}

	return false
}

func (m *ModuleControl) Log(event *audit.Event) error {
	eventLog := NewModuleEventLog(event)
	eventLog.Type = "Module control"

	eventLog.Component = event.ObjectRef.Name

	switch event.Verb {
	case "create":
		eventLog.Name = "Module creation"
		eventLog.Level = "info"
	case "patch", "update":
		eventLog.Name = "Module update"
		eventLog.Level = "info"
	case "delete":
		eventLog.Name = "Module deletion"
		eventLog.Level = "warn"
	}

	moduleConfig, err := getModuleConfigFromInformer(m.moduleConfigInformer, event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get moduleconfig from informer", log.Err(err))

		return eventLog.Log()
	}

	if (event.Verb == "patch" || event.Verb == "update") || !*moduleConfig.Spec.Enabled {
		eventLog.Name = "Module disable"
		eventLog.Level = "warn"
	}

	module, err := getModuleFromInformer(m.moduleInformer, event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get module from informer", log.Err(err))
	}

	if module != nil {
		eventLog.VirtualizationVersion = module.Properties.Version
	}

	return eventLog.Log()
}
