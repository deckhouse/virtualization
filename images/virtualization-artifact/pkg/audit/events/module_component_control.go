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
		informerList: options.InformerList,
		ttlCache:     options.TTLCache,
	}
}

type ModuleComponentControl struct {
	informerList informerList
	ttlCache     ttlCache
}

func (m *ModuleComponentControl) IsMatched(event *audit.Event) bool {
	if (event.ObjectRef == nil && event.ObjectRef.Name != "") || event.Stage != audit.StageResponseComplete {
		return false
	}

	// Skip control requests from internal k8s controllers because we get them with almost empty ObjectRef
	if strings.Contains(event.User.Username, "system:serviceaccount:kube-system") {
		return false
	}

	if strings.Contains(event.ObjectRef.Name, "cvi-importer") {
		return false
	}

	if (event.Verb == "delete" || event.Verb == "create") &&
		event.ObjectRef.Resource == "pods" &&
		event.ObjectRef.Namespace == "d8-virtualization" {
		return true
	}

	return false
}

func (m *ModuleComponentControl) Log(event *audit.Event) error {
	eventLog := NewModuleEventLog(event)
	eventLog.Type = "Virtualization control"

	if event.Verb == "create" {
		eventLog.Name = "Component creation"
		eventLog.Level = "info"
		eventLog.Component = event.ObjectRef.Name
	} else {
		eventLog.Name = "Component deletion"
		eventLog.Level = "warn"
		eventLog.Component = event.ObjectRef.Name
	}

	pod, err := getPodFromInformer(m.ttlCache, m.informerList.GetPodInformer(), event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get pod from informer", log.Err(err))

		return eventLog.Log()
	}

	err = eventLog.fillNodeInfo(m.informerList.GetNodeInformer(), pod)
	if err != nil {
		log.Debug("fail to fill node info", log.Err(err))
	}

	module, err := getModuleFromInformer(m.informerList.GetModuleInformer(), "virtualization")
	if err != nil {
		log.Debug("fail to get module from informer", log.Err(err))
	}

	if module != nil {
		eventLog.VirtualizationVersion = module.Properties.Version
	}

	return eventLog.Log()
}
