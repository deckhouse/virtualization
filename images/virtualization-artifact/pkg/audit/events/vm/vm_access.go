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
	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
)

func NewVMAccess(options events.EventLoggerOptions) *VMAccess {
	return &VMAccess{
		Event:        options.GetEvent(),
		InformerList: options.GetInformerList(),
		TTLCache:     options.GetTTLCache(),
	}
}

type VMAccess struct {
	Event        *audit.Event
	EventLog     *VMEventLog
	InformerList events.InformerList
	TTLCache     events.TTLCache
}

func (m *VMAccess) Log() error {
	return m.EventLog.Log()
}

func (m *VMAccess) ShouldLog() bool {
	return m.EventLog.shouldLog
}

func (m *VMAccess) IsMatched() bool {
	if m.Event.ObjectRef == nil {
		return false
	}

	if m.Event.Verb != "get" {
		return false
	}

	if m.Event.ObjectRef.Resource != "virtualmachines" || m.Event.ObjectRef.APIGroup != "subresources.virtualization.deckhouse.io" {
		return false
	}

	if m.Event.ObjectRef.Subresource == "console" || m.Event.ObjectRef.Subresource == "vnc" || m.Event.ObjectRef.Subresource == "portforward" {
		return true
	}

	return false
}

func (m *VMAccess) Fill() error {
	m.EventLog = NewVMEventLog(m.Event)
	m.EventLog.Type = "Access to VM"

	switch m.Event.ObjectRef.Subresource {
	case "console":
		m.EventLog.Name = "Access to VM via serial console"
	case "vnc":
		m.EventLog.Name = "Access to VM via VNC"
	case "portforward":
		m.EventLog.Name = "Access to VM via portforward"
	}

	if m.Event.Stage == audit.StageRequestReceived {
		m.EventLog.Name = "Request " + m.EventLog.Name
	}

	vm, err := util.GetVMFromInformer(m.TTLCache, m.InformerList.GetVMInformer(), m.Event.ObjectRef.Namespace+"/"+m.Event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get vm from informer", log.Err(err))

		return nil
	}

	m.EventLog.VirtualmachineUID = string(vm.UID)

	if vm.Status.GuestOSInfo.Name != "" {
		m.EventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name
	}

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
