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

package module

import (
	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
)

func NewModuleControl(options events.EventLoggerOptions) *ModuleControl {
	return &ModuleControl{
		Event:        options.GetEvent(),
		InformerList: options.GetInformerList(),
	}
}

type ModuleControl struct {
	Event        *audit.Event
	EventLog     *ModuleEventLog
	InformerList events.InformerList
}

func (m *ModuleControl) Log() error {
	return m.EventLog.Log()
}

func (m *ModuleControl) ShouldLog() bool {
	return m.EventLog.shouldLog
}

func (m *ModuleControl) IsMatched() bool {
	if m.Event.ObjectRef == nil || m.Event.Stage != audit.StageResponseComplete {
		return false
	}

	if m.Event.Verb == "get" || m.Event.Verb == "list" {
		return false
	}

	if m.Event.ObjectRef.Resource == "moduleconfigs" {
		return true
	}

	return false
}

func (m *ModuleControl) Fill() error {
	m.EventLog = NewModuleEventLog(m.Event)
	m.EventLog.Type = "Module control"

	m.EventLog.Component = m.Event.ObjectRef.Name

	switch m.Event.Verb {
	case "create":
		m.EventLog.Name = "Module creation"
		m.EventLog.Level = "info"
	case "patch", "update":
		m.EventLog.Name = "Module update"
		m.EventLog.Level = "info"
	case "delete":
		m.EventLog.Name = "Module deletion"
		m.EventLog.Level = "warn"
	}

	moduleConfig, err := util.GetModuleConfigFromInformer(m.InformerList.GetModuleConfigInformer(), m.Event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get moduleconfig from informer", log.Err(err))
		return nil
	}

	if (m.Event.Verb == "patch" || m.Event.Verb == "update") && !*moduleConfig.Spec.Enabled {
		m.EventLog.Name = "Module disabled"
		m.EventLog.Level = "warn"
	}

	module, err := util.GetModuleFromInformer(m.InformerList.GetModuleInformer(), m.Event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get module from informer", log.Err(err))
	}

	if module != nil {
		m.EventLog.VirtualizationVersion = module.Properties.Version
	}

	return nil
}
