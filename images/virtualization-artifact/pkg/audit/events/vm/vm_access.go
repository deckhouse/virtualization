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

func NewVMAccess(options events.EventLoggerOptions) events.EventLogger {
	return &VMAccess{
		event:        options.GetEvent(),
		informerList: options.GetInformerList(),
		ttlCache:     options.GetTTLCache(),
	}
}

type VMAccess struct {
	event        *audit.Event
	eventLog     *VMEventLog
	informerList events.InformerList
	ttlCache     events.TTLCache
}

func (m *VMAccess) Log() error {
	return m.eventLog.Log()
}

func (m *VMAccess) ShouldLog() bool {
	return m.eventLog.shouldLog
}

func (m *VMAccess) IsMatched() bool {
	if m.event.ObjectRef == nil {
		return false
	}

	if m.event.ObjectRef.Resource != "virtualmachines" || m.event.ObjectRef.APIGroup != "subresources.virtualization.deckhouse.io" {
		return false
	}

	if m.event.ObjectRef.Subresource == "console" || m.event.ObjectRef.Subresource == "vnc" || m.event.ObjectRef.Subresource == "portforward" {
		return true
	}

	return false
}

func (m *VMAccess) Fill() error {
	m.eventLog = NewVMEventLog(m.event)
	m.eventLog.Type = "Access to VM"

	switch m.event.ObjectRef.Subresource {
	case "console":
		m.eventLog.Name = "Access to VM via serial console"
	case "vnc":
		m.eventLog.Name = "Access to VM via VNC"
	case "portforward":
		m.eventLog.Name = "Access to VM via portforward"
	}

	if m.event.Stage == audit.StageRequestReceived {
		m.eventLog.Name = "Request " + m.eventLog.Name
	}

	vm, err := util.GetVMFromInformer(m.ttlCache, m.informerList.GetVMInformer(), m.event.ObjectRef.Namespace+"/"+m.event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get vm from informer", log.Err(err))

		return nil
	}

	m.eventLog.VirtualmachineUID = string(vm.UID)

	if vm.Status.GuestOSInfo.Name != "" {
		m.eventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name
	}

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
