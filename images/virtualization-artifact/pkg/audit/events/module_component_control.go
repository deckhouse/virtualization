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
	"fmt"
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type NewModuleComponentControlOptions struct {
	NodeInformer indexer
	PodInformer  indexer
	TTLCache     ttlCache
}

func NewModuleComponentControl(options NewModuleComponentControlOptions) *ModuleComponentControl {
	return &ModuleComponentControl{
		nodeInformer: options.NodeInformer,
		podInformer:  options.PodInformer,
		ttlCache:     options.TTLCache,
	}
}

type ModuleComponentControl struct {
	podInformer  indexer
	nodeInformer indexer
	ttlCache     ttlCache
}

func (m *ModuleComponentControl) IsMatched(event *audit.Event) bool {
	if (event.ObjectRef == nil && event.ObjectRef.Name != "") || event.Stage != audit.StageResponseComplete {
		return false
	}

	// Skip control requests from internal k8s controllers because we get them with almost empty ObjectRef
	if event.User.Username == "system:serviceaccount:kube-system:daemon-set-controller" ||
		event.User.Username == "system:serviceaccount:kube-system:statefulset-controller" ||
		event.User.Username == "system:serviceaccount:kube-system:replicaset-controller" {
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

	pod, err := getPodFromInformer(m.ttlCache, m.podInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod from informer: %w", err)
	}

	err = eventLog.fillNodeInfo(m.nodeInformer, pod)
	if err != nil {
		log.Debug("fail to fill node info", log.Err(err))
	}

	if event.Verb == "create" {
		eventLog.Name = "Component creation"
		eventLog.Level = "info"
		eventLog.Component = pod.Name
	} else {
		eventLog.Name = "Component deletion"
		eventLog.Level = "warn"
		eventLog.Component = pod.Name
	}

	return eventLog.Log()
}
