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
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMOPControl(options events.EventLoggerOptions) *VMOPControl {
	return &VMOPControl{
		event:        options.GetEvent(),
		informerList: options.GetInformerList(),
		ttlCache:     options.GetTTLCache(),
	}
}

type VMOPControl struct {
	eventLog     *VMEventLog
	event        *audit.Event
	informerList events.InformerList
	ttlCache     events.TTLCache
}

func (m *VMOPControl) Log() error {
	return m.eventLog.Log()
}

func (m *VMOPControl) ShouldLog() bool {
	return true
}

func (m *VMOPControl) IsMatched() bool {
	if m.event.ObjectRef == nil || m.event.Stage != audit.StageResponseComplete {
		return false
	}

	if m.event.Level != audit.LevelRequestResponse {
		return false
	}

	if strings.HasPrefix(m.event.User.Username, "system:") &&
		!strings.HasPrefix(m.event.User.Username, "system:serviceaccount:d8-service-accounts") {
		return false
	}

	if m.event.ObjectRef.Resource == "virtualmachineoperations" && m.event.Verb == "create" {
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
	m.eventLog = NewVMEventLog(m.event)
	m.eventLog.Type = "Control VM"

	var response vmopResponseObject
	err := json.Unmarshal(m.event.ResponseObject.Raw, &response)
	if err != nil {
		return fmt.Errorf("fail to unmarshal event ResponseObject: %w", err)
	}

	vmop, err := util.GetVMOPFromInformer(m.informerList.GetVMOPInformer(), m.event.ObjectRef.Namespace+"/"+response.Metadata.Name)
	if err != nil {
		return fmt.Errorf("fail to get vmop from informer: %w", err)
	}

	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been started by '%s'", vmop.Spec.VirtualMachine, m.event.User.Username)
		m.eventLog.Level = "info"
		m.eventLog.ActionType = "start"
	case v1alpha2.VMOPTypeStop:
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been stopped by '%s'", vmop.Spec.VirtualMachine, m.event.User.Username)
		m.eventLog.Level = "warn"
		m.eventLog.ActionType = "stop"
	case v1alpha2.VMOPTypeRestart:
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been restarted by '%s'", vmop.Spec.VirtualMachine, m.event.User.Username)
		m.eventLog.Level = "warn"
		m.eventLog.ActionType = "restart"
	case v1alpha2.VMOPTypeMigrate:
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been migrated by '%s'", vmop.Spec.VirtualMachine, m.event.User.Username)
		m.eventLog.Level = "warn"
		m.eventLog.ActionType = "migrate"
	case v1alpha2.VMOPTypeEvict:
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been evicted by '%s'", vmop.Spec.VirtualMachine, m.event.User.Username)
		m.eventLog.Level = "warn"
		m.eventLog.ActionType = "evict"
	}

	vm, err := util.GetVMFromInformer(m.ttlCache, m.informerList.GetVMInformer(), vmop.Namespace+"/"+vmop.Spec.VirtualMachine)
	if err != nil {
		return fmt.Errorf("fail to get vm from informer: %w", err)
	}

	m.eventLog.QemuVersion = vm.Status.Versions.Qemu
	m.eventLog.LibvirtVersion = vm.Status.Versions.Libvirt

	m.eventLog.VirtualmachineUID = string(vm.UID)
	m.eventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name

	if len(vm.Spec.BlockDeviceRefs) > 0 {
		if err := m.eventLog.fillVDInfo(m.ttlCache, m.informerList.GetVDInformer(), vm); err != nil {
			log.Debug("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := m.eventLog.fillNodeInfo(m.informerList.GetNodeInformer(), vm); err != nil {
			log.Debug("fail to fill node info", log.Err(err))
		}
	}

	return nil
}
