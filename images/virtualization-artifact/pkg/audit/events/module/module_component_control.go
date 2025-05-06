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
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

const (
	virtuliaztionNamespace = "d8-virtualization"
	kubeSystemUsername     = "system:serviceaccount:kube-system"
	cviImporterName        = "cvi-importer"
)

func NewModuleComponentControl(options events.EventLoggerOptions) *ModuleComponentControl {
	return &ModuleComponentControl{
		Event:        options.GetEvent(),
		InformerList: options.GetInformerList(),
		TTLCache:     options.GetTTLCache(),
	}
}

type ModuleComponentControl struct {
	Event        *audit.Event
	EventLog     *ModuleEventLog
	InformerList events.InformerList
	TTLCache     events.TTLCache
}

func (m *ModuleComponentControl) Log() error {
	return m.EventLog.Log()
}

func (m *ModuleComponentControl) ShouldLog() bool {
	return m.EventLog.shouldLog
}

func (m *ModuleComponentControl) IsMatched() bool {
	if m.Event.ObjectRef == nil || m.Event.ObjectRef.Name == "" || m.Event.Stage != audit.StageResponseComplete {
		return false
	}

	// Skip control requests from internal k8s controllers because we get them with almost empty ObjectRef
	if strings.Contains(m.Event.User.Username, kubeSystemUsername) {
		return false
	}

	if strings.Contains(m.Event.ObjectRef.Name, cviImporterName) {
		return false
	}

	if (m.Event.Verb == "delete" || m.Event.Verb == "create") &&
		m.Event.ObjectRef.Resource == "pods" &&
		m.Event.ObjectRef.Namespace == virtuliaztionNamespace {
		return true
	}

	return false
}

func (m *ModuleComponentControl) Fill() error {
	m.EventLog = NewModuleEventLog(m.Event)
	m.EventLog.Type = "Virtualization control"

	if m.Event.Verb == "create" {
		m.EventLog.Name = "Component creation"
		m.EventLog.Level = "info"
		m.EventLog.Component = m.Event.ObjectRef.Name
	} else {
		m.EventLog.Name = "Component deletion"
		m.EventLog.Level = "warn"
		m.EventLog.Component = m.Event.ObjectRef.Name
	}

	pod, err := util.GetPodFromInformer(m.TTLCache, m.InformerList.GetPodInformer(), m.Event.ObjectRef.Namespace+"/"+m.Event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get pod from informer", log.Err(err))
		return nil
	}

	m.EventLog.QemuVersion = pod.Annotations[annotations.AnnQemuVersion]
	m.EventLog.LibvirtVersion = pod.Annotations[annotations.AnnLibvirtVersion]

	err = m.EventLog.fillNodeInfo(m.InformerList.GetNodeInformer(), pod)
	if err != nil {
		log.Debug("fail to fill node info", log.Err(err))
	}

	module, err := util.GetModuleFromInformer(m.InformerList.GetModuleInformer(), "virtualization")
	if err != nil {
		log.Debug("fail to get module from informer", log.Err(err))
	}

	if module != nil {
		m.EventLog.VirtualizationVersion = module.Properties.Version
	}

	return nil
}
