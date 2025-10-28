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
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

const (
	virtuliaztionNamespace = "d8-virtualization"
	kubeSystemUsername     = "system:serviceaccount:kube-system"
	cviImporterName        = "cvi-importer"
)

func NewModuleComponentControl(options events.EventLoggerOptions) *ModuleComponentControl {
	return &ModuleComponentControl{
		event:        options.GetEvent(),
		informerList: options.GetInformerList(),
		ttlCache:     options.GetTTLCache(),
	}
}

type ModuleComponentControl struct {
	event        *audit.Event
	eventLog     *ModuleEventLog
	informerList events.InformerList
	ttlCache     events.TTLCache
}

func (m *ModuleComponentControl) Log() error {
	return m.eventLog.Log()
}

func (m *ModuleComponentControl) ShouldLog() bool {
	return m.eventLog.shouldLog
}

func (m *ModuleComponentControl) IsMatched() bool {
	if m.event.ObjectRef == nil || m.event.ObjectRef.Name == "" || m.event.Stage != audit.StageResponseComplete {
		return false
	}

	if strings.HasPrefix(m.event.User.Username, "system:") &&
		!strings.HasPrefix(m.event.User.Username, "system:serviceaccount:d8-service-accounts") {
		return false
	}

	if strings.Contains(m.event.ObjectRef.Name, cviImporterName) {
		return false
	}

	if (m.event.Verb == "delete" || m.event.Verb == "create") &&
		m.event.ObjectRef.Resource == "pods" &&
		m.event.ObjectRef.Namespace == virtuliaztionNamespace {
		return true
	}

	return false
}

func (m *ModuleComponentControl) Fill() error {
	m.eventLog = NewModuleEventLog(m.event)
	m.eventLog.Type = "Virtualization control"

	if m.event.Verb == "create" {
		m.eventLog.Name = fmt.Sprintf("Component '%s' has been created by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
		m.eventLog.Level = "info"
		m.eventLog.Component = m.event.ObjectRef.Name
	} else {
		m.eventLog.Name = fmt.Sprintf("Component '%s' has been deleted by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
		m.eventLog.Level = "warn"
		m.eventLog.Component = m.event.ObjectRef.Name
	}

	pod, err := util.GetPodFromInformer(m.ttlCache, m.informerList.GetPodInformer(), m.event.ObjectRef.Namespace+"/"+m.event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get pod from informer", log.Err(err))
		return nil
	}

	m.eventLog.QemuVersion = pod.Annotations[annotations.AnnQemuVersion]
	m.eventLog.LibvirtVersion = pod.Annotations[annotations.AnnLibvirtVersion]

	err = m.eventLog.fillNodeInfo(m.informerList.GetNodeInformer(), pod)
	if err != nil {
		log.Debug("fail to fill node info", log.Err(err))
	}

	module, err := util.GetModuleFromInformer(m.informerList.GetModuleInformer(), "virtualization")
	if err != nil {
		log.Debug("fail to get module from informer", log.Err(err))
	}

	if module != nil {
		m.eventLog.VirtualizationVersion = module.Properties.Version
	}

	return nil
}
