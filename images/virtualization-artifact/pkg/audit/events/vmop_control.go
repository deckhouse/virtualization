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
	"encoding/json"
	"fmt"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMOPControl(options NewEventHandlerOptions) eventLogger {
	return &VMOPControl{
		informerList: options.InformerList,
		ttlCache:     options.TTLCache,
	}
}

type VMOPControl struct {
	informerList informerList
	ttlCache     ttlCache
}

func (m *VMOPControl) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if event.ObjectRef.Resource == "virtualmachineoperations" && event.Verb == "create" {
		return true
	}

	return false
}

type vmopResponseObject struct {
	Metadata vmopResponseObjectMetadata `json:"metadata"`
}

type vmopResponseObjectMetadata struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func (m *VMOPControl) Log(event *audit.Event) error {
	eventLog := NewVMEventLog(event)
	eventLog.Type = "Control VM"

	var response vmopResponseObject
	err := json.Unmarshal(event.ResponseObject.Raw, &response)
	if err != nil {
		return fmt.Errorf("fail to unmarshal event ResponseObject: %w", err)
	}

	vmop, err := getVMOPFromInformer(m.informerList.GetVMOPInformer(), event.ObjectRef.Namespace+"/"+response.Metadata.Name)
	if err != nil {
		return fmt.Errorf("fail to get vmop from informer: %w", err)
	}

	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		eventLog.Name = "VM started"
		eventLog.Level = "info"
	case v1alpha2.VMOPTypeStop:
		eventLog.Name = "VM stopped"
		eventLog.Level = "warn"
	case v1alpha2.VMOPTypeRestart:
		eventLog.Name = "VM restarted"
		eventLog.Level = "warn"
	case v1alpha2.VMOPTypeMigrate:
		eventLog.Name = "VM migrated"
		eventLog.Level = "warn"
	case v1alpha2.VMOPTypeEvict:
		eventLog.Name = "VM evicted"
		eventLog.Level = "warn"
	}

	vm, err := getVMFromInformer(m.ttlCache, m.informerList.GetVMInformer(), vmop.Namespace+"/"+vmop.Spec.VirtualMachine)
	if err != nil {
		return fmt.Errorf("fail to get vm from informer: %w", err)
	}

	eventLog.VirtualmachineUID = string(vm.UID)
	eventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name

	if len(vm.Spec.BlockDeviceRefs) > 0 {
		if err := eventLog.fillVDInfo(m.ttlCache, m.informerList.GetVDInformer(), vm); err != nil {
			log.Debug("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := eventLog.fillNodeInfo(m.informerList.GetNodeInformer(), vm); err != nil {
			log.Debug("fail to fill node info", log.Err(err))
		}
	}

	return eventLog.Log()
}
