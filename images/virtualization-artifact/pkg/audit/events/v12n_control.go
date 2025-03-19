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
	"k8s.io/client-go/tools/cache"
)

type NewV12NControlOptions struct {
	NodeInformer cache.Indexer
	PodInformer  cache.Indexer
}

func NewV12NControl(options NewV12NControlOptions) *V12NControl {
	return &V12NControl{
		nodeInformer: options.NodeInformer,
		podInformer:  options.PodInformer,
	}
}

type V12NControl struct {
	podInformer  cache.Indexer
	nodeInformer cache.Indexer
}

func (m *V12NControl) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if (event.Verb == "delete" || event.Verb == "create") &&
		event.ObjectRef.Resource == "pods" &&
		event.ObjectRef.Namespace == "d8-virtualization" {
		return true
	}

	return false
}

func (m *V12NControl) Log(event *audit.Event) error {
	eventLog := NewV12NEventLog(event)
	eventLog.Type = "Virtualization control"

	pod, err := getPodFromInformer(m.podInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod from informer: %w", err)
	}

	err = eventLog.fillNodeInfo(m.nodeInformer, pod)
	if err != nil {
		return fmt.Errorf("fail to fill node info: %w", err)
	}

	if event.Verb == "create" {
		eventLog.Name = "Component creation"
		eventLog.Level = "info"

		if strings.Contains(pod.Name, "virt-handler") {
		} else {
		}
	} else {
		eventLog.Name = "Component deletion"
		eventLog.Level = "warn"

		if strings.Contains(pod.Name, "virt-handler") {
		} else {
		}
	}

	return eventLog.Log()
}
