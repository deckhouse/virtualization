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
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
)

func NewModuleComponentControl(options NewEventHandlerOptions) eventLogger {
	return &ModuleComponentControl{
		event:        options.Event,
		informerList: options.InformerList,
		ttlCache:     options.TTLCache,
	}
}

type ModuleComponentControl struct {
	event        *audit.Event
	eventLog     *ModuleEventLog
	informerList informerList
	ttlCache     ttlCache
}

func (m *ModuleComponentControl) Log() error {
	return m.eventLog.Log()
}

func (m *ModuleComponentControl) ShouldLog() bool {
	return m.eventLog.shouldLog
}

func (m *ModuleComponentControl) IsMatched() bool {
	if (m.event.ObjectRef == nil && m.event.ObjectRef.Name != "") || m.event.Stage != audit.StageResponseComplete {
		return false
	}

	// Skip control requests from internal k8s controllers because we get them with almost empty ObjectRef
	if strings.Contains(m.event.User.Username, "system:serviceaccount:kube-system") {
		return false
	}

	if strings.Contains(m.event.ObjectRef.Name, "cvi-importer") {
		return false
	}

	if (m.event.Verb == "delete" || m.event.Verb == "create") &&
		m.event.ObjectRef.Resource == "pods" &&
		m.event.ObjectRef.Namespace == "d8-virtualization" {
		return true
	}

	return false
}

func (m *ModuleComponentControl) Fill() error {
	m.eventLog = NewModuleEventLog(m.event)
	m.eventLog.Type = "Virtualization control"

	if m.event.Verb == "create" {
		m.eventLog.Name = "Component creation"
		m.eventLog.Level = "info"
		m.eventLog.Component = m.event.ObjectRef.Name
	} else {
		m.eventLog.Name = "Component deletion"
		m.eventLog.Level = "warn"
		m.eventLog.Component = m.event.ObjectRef.Name
	}

	pod, err := getPodFromInformer(m.ttlCache, m.informerList.GetPodInformer(), m.event.ObjectRef.Namespace+"/"+m.event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get pod from informer", log.Err(err))

		return m.eventLog.Log()
	}

	err = m.eventLog.fillNodeInfo(m.informerList.GetNodeInformer(), pod)
	if err != nil {
		log.Debug("fail to fill node info", log.Err(err))
	}

	module, err := getModuleFromInformer(m.informerList.GetModuleInformer(), "virtualization")
	if err != nil {
		log.Debug("fail to get module from informer", log.Err(err))
	}

	if module != nil {
		m.eventLog.VirtualizationVersion = module.Properties.Version
	}

	return nil
}
