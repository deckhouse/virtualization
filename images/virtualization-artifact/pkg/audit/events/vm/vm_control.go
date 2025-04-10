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
	"fmt"
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
)

func NewVMControl(options events.EventLoggerOptions) *VMControl {
	return &VMControl{
		Event:        options.GetEvent(),
		InformerList: options.GetInformerList(),
		TTLCache:     options.GetTTLCache(),
	}
}

type VMControl struct {
	Event        *audit.Event
	EventLog     *VMEventLog
	InformerList events.InformerList
	TTLCache     events.TTLCache
}

func (m *VMControl) Log() error {
	return m.EventLog.Log()
}

func (m *VMControl) ShouldLog() bool {
	return m.EventLog.shouldLog
}

func (m *VMControl) IsMatched() bool {
	if m.Event.ObjectRef == nil || m.Event.Stage != audit.StageResponseComplete {
		return false
	}

	if strings.Contains(m.Event.ObjectRef.Name, "virt-launcher") && m.Event.ObjectRef.Resource == "pods" && m.Event.Verb == "delete" {
		return true
	}

	return false
}

func (m *VMControl) Fill() error {
	m.EventLog = NewVMEventLog(m.Event)
	m.EventLog.Type = "Control VM"

	pod, err := util.GetPodFromInformer(m.TTLCache, m.InformerList.GetPodInformer(), m.Event.ObjectRef.Namespace+"/"+m.Event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod from informer: %w", err)
	}

	var terminatedStatuses string
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "compute" && status.State.Terminated != nil {
			terminatedStatuses = status.State.Terminated.Message
		}
	}

	isControllerAction := strings.Contains(m.Event.User.Username, "system:serviceaccount:d8-virtualization")
	isNodeAction := strings.Contains(m.Event.User.Username, "system:node")

	switch {
	case isControllerAction:
		m.EventLog.Level = "warn"

		switch {
		case strings.Contains(terminatedStatuses, "guest-shutdown"):
			m.EventLog.Name = "VM stoped from OS"
		case strings.Contains(terminatedStatuses, "guest-reset"):
			m.EventLog.Name = "VM restarted from OS"
		default:
			m.EventLog.Name = "VM stopped by system"
			return nil
		}
	case isNodeAction:
		m.EventLog.Name = "VM stopped by system"
		return nil
	default:
		m.EventLog.Level = "critical"
		m.EventLog.Name = "VM killed abnormal way"
	}

	vm, err := util.GetVMFromInformer(m.TTLCache, m.InformerList.GetVMInformer(), pod.Namespace+"/"+pod.Labels["vm.kubevirt.internal.virtualization.deckhouse.io/name"])
	if err != nil {
		log.Debug("fail to get vm from informer", log.Err(err))
		return nil
	}

	m.EventLog.QemuVersion = vm.Status.Versions.Qemu
	m.EventLog.LibvirtVersion = vm.Status.Versions.Libvirt

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
