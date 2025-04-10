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

package vm

import (
	"encoding/json"
	"fmt"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMOPControl(options events.EventLoggerOptions) *VMOPControl {
	return &VMOPControl{
		Event:        options.GetEvent(),
		InformerList: options.GetInformerList(),
		TTLCache:     options.GetTTLCache(),
	}
}

type VMOPControl struct {
	EventLog     *VMEventLog
	Event        *audit.Event
	InformerList events.InformerList
	TTLCache     events.TTLCache
}

func (m *VMOPControl) Log() error {
	return m.EventLog.Log()
}

func (m *VMOPControl) ShouldLog() bool {
	return true
}

func (m *VMOPControl) IsMatched() bool {
	if m.Event.ObjectRef == nil || m.Event.Stage != audit.StageResponseComplete {
		return false
	}

	if m.Event.Level != audit.LevelRequestResponse {
		return false
	}

	if m.Event.ObjectRef.Resource == "virtualmachineoperations" && m.Event.Verb == "create" {
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

func (m *VMOPControl) Fill() error {
	m.EventLog = NewVMEventLog(m.Event)
	m.EventLog.Type = "Control VM"

	var response vmopResponseObject
	err := json.Unmarshal(m.Event.ResponseObject.Raw, &response)
	if err != nil {
		return fmt.Errorf("fail to unmarshal event ResponseObject: %w", err)
	}

	vmop, err := util.GetVMOPFromInformer(m.InformerList.GetVMOPInformer(), m.Event.ObjectRef.Namespace+"/"+response.Metadata.Name)
	if err != nil {
		return fmt.Errorf("fail to get vmop from informer: %w", err)
	}

	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		m.EventLog.Name = "VM started"
		m.EventLog.Level = "info"
		m.EventLog.ActionType = "start"
	case v1alpha2.VMOPTypeStop:
		m.EventLog.Name = "VM stopped"
		m.EventLog.Level = "warn"
		m.EventLog.ActionType = "stop"
	case v1alpha2.VMOPTypeRestart:
		m.EventLog.Name = "VM restarted"
		m.EventLog.Level = "warn"
		m.EventLog.ActionType = "restart"
	case v1alpha2.VMOPTypeMigrate:
		m.EventLog.Name = "VM migrated"
		m.EventLog.Level = "warn"
		m.EventLog.ActionType = "migrate"
	case v1alpha2.VMOPTypeEvict:
		m.EventLog.Name = "VM evicted"
		m.EventLog.Level = "warn"
		m.EventLog.ActionType = "evict"
	}

	vm, err := util.GetVMFromInformer(m.TTLCache, m.InformerList.GetVMInformer(), vmop.Namespace+"/"+vmop.Spec.VirtualMachine)
	if err != nil {
		return fmt.Errorf("fail to get vm from informer: %w", err)
	}

	m.EventLog.QemuVersion = vm.Status.Versions.Qemu
	m.EventLog.LibvirtVersion = vm.Status.Versions.Libvirt

	m.EventLog.VirtualmachineUID = string(vm.UID)
	m.EventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name

	if len(vm.Spec.BlockDeviceRefs) > 0 {
		if err := m.EventLog.fillVDInfo(m.TTLCache, m.InformerList.GetVDInformer(), vm); err != nil {
			log.Debug("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := m.EventLog.fillNodeInfo(m.InformerList.GetNodeInformer(), vm); err != nil {
			log.Debug("fail to fill node info", log.Err(err))
		}
	}

	return nil
}
