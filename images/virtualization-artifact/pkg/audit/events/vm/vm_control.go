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
	vmutil "github.com/deckhouse/virtualization-controller/pkg/common/vm"
)

func NewVMControl(options events.EventLoggerOptions) *VMControl {
	return &VMControl{
		event:        options.GetEvent(),
		informerList: options.GetInformerList(),
		ttlCache:     options.GetTTLCache(),
	}
}

type VMControl struct {
	event        *audit.Event
	eventLog     *VMEventLog
	informerList events.InformerList
	ttlCache     events.TTLCache
}

func (m *VMControl) Log() error {
	return m.eventLog.Log()
}

func (m *VMControl) ShouldLog() bool {
	return m.eventLog.shouldLog
}

func (m *VMControl) IsMatched() bool {
	if m.event.ObjectRef == nil || m.event.Stage != audit.StageResponseComplete {
		return false
	}

	if strings.Contains(m.event.ObjectRef.Name, "virt-launcher") && m.event.ObjectRef.Resource == "pods" && m.event.Verb == "delete" {
		return true
	}

	return false
}

func (m *VMControl) Fill() error {
	m.eventLog = NewVMEventLog(m.event)
	m.eventLog.Type = "Control VM"

	pod, err := util.GetPodFromInformer(m.ttlCache, m.informerList.GetPodInformer(), m.event.ObjectRef.Namespace+"/"+m.event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod from informer: %w", err)
	}

	var terminatedStatuses string
	for _, status := range pod.Status.ContainerStatuses {
		if vmutil.IsComputeContainer(status.Name) && status.State.Terminated != nil {
			terminatedStatuses = status.State.Terminated.Message
		}
	}

	vmName := pod.Labels["vm.kubevirt.internal.virtualization.deckhouse.io/name"]
	isControllerAction := strings.HasPrefix(m.event.User.Username, "system:serviceaccount:d8-virtualization")
	isNodeAction := strings.HasPrefix(m.event.User.Username, "system:node")

	switch {
	case isControllerAction:
		m.eventLog.Level = "warn"

		switch {
		case strings.Contains(terminatedStatuses, "guest-shutdown"):
			m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been stopped from OS", vmName)
		case strings.Contains(terminatedStatuses, "guest-reset"):
			m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been restarted from OS", vmName)
		default:
			m.eventLog.shouldLog = false
			return nil
		}
	case isNodeAction:
		m.eventLog.shouldLog = false
		return nil
	default:
		m.eventLog.Level = "critical"
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been killed abnormal way by '%s'", vmName, m.event.User.Username)
	}

	vm, err := util.GetVMFromInformer(m.ttlCache, m.informerList.GetVMInformer(), pod.Namespace+"/"+vmName)
	if err != nil {
		log.Debug("fail to get vm from informer", log.Err(err))
		return nil
	}

	m.eventLog.QemuVersion = vm.Status.Versions.Qemu
	m.eventLog.LibvirtVersion = vm.Status.Versions.Libvirt

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
