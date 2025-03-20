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
)

type NewV12NModuleControlOptions struct {
	NodeInformer indexer
}

func NewV12NModuleControl(options NewV12NModuleControlOptions) *V12NModuleControl {
	return &V12NModuleControl{
		nodeInformer: options.NodeInformer,
	}
}

type V12NModuleControl struct {
	nodeInformer indexer
}

func (m *V12NModuleControl) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if event.ObjectRef.Resource == "moduleconfigs" && event.ObjectRef.Name == "virtualization" {
		return true
	}

	return false
}

func (m *V12NModuleControl) Log(event *audit.Event) error {
	eventLog := NewV12NEventLog(event)
	eventLog.Type = "Virtualization control"

	switch event.Verb {
	case "create":
		eventLog.Name = "Module creation"
		eventLog.Level = "info"
	case "update":
		eventLog.Name = "Module update"
		eventLog.Level = "info"

		// if module.enabled = false {
		// 	eventLog.Name = "Module disable"
		// 	eventLog.Level = "warn"
		// }
	case "delete":
		eventLog.Name = "Module deletion"
		eventLog.Level = "warn"
	}

	return eventLog.Log()
}
