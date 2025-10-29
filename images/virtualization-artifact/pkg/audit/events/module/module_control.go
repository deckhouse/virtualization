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
	"fmt"
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
)

func NewModuleControl(options events.EventLoggerOptions) *ModuleControl {
	return &ModuleControl{
		event:        options.GetEvent(),
		informerList: options.GetInformerList(),
	}
}

type ModuleControl struct {
	event        *audit.Event
	eventLog     *ModuleEventLog
	informerList events.InformerList
}

func (m *ModuleControl) Log() error {
	return m.eventLog.Log()
}

func (m *ModuleControl) ShouldLog() bool {
	return m.eventLog.shouldLog
}

func (m *ModuleControl) IsMatched() bool {
	if m.event.ObjectRef == nil || m.event.Stage != audit.StageResponseComplete {
		return false
	}

	if m.event.Verb != "create" && m.event.Verb != "patch" && m.event.Verb != "update" && m.event.Verb != "delete" {
		return false
	}

	if strings.HasPrefix(m.event.User.Username, "system:") &&
		!strings.HasPrefix(m.event.User.Username, "system:serviceaccount:d8-service-accounts") {
		return false
	}

	if m.event.ObjectRef.Resource == "moduleconfigs" {
		return true
	}

	return false
}

func (m *ModuleControl) Fill() error {
	m.eventLog = NewModuleEventLog(m.event)
	m.eventLog.Type = "Module control"

	m.eventLog.Component = m.event.ObjectRef.Name

	switch m.event.Verb {
	case "create":
		m.eventLog.Name = fmt.Sprintf("Module '%s' has been created by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
		m.eventLog.Level = "info"
	case "patch", "update":
		m.eventLog.Name = fmt.Sprintf("Module '%s' has been updated by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
		m.eventLog.Level = "info"
	case "delete":
		m.eventLog.Name = fmt.Sprintf("Module '%s' has been deleted by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
		m.eventLog.Level = "warn"
	}

	moduleConfig, err := util.GetModuleConfigFromInformer(m.informerList.GetModuleConfigInformer(), m.event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get moduleconfig from informer", log.Err(err))
		return nil
	}

	if (m.event.Verb == "patch" || m.event.Verb == "update") && (moduleConfig.Spec.Enabled != nil && !*moduleConfig.Spec.Enabled) {
		m.eventLog.Name = fmt.Sprintf("Module '%s' has been disabled by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
		m.eventLog.Level = "warn"
	}

	module, err := util.GetModuleFromInformer(m.informerList.GetModuleInformer(), m.event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get module from informer", log.Err(err))
	}

	if module != nil {
		m.eventLog.VirtualizationVersion = module.Properties.Version
	}

	return nil
}
