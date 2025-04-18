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
	"log/slog"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
)

func NewVMManage(options events.EventLoggerOptions) *VMManage {
	return &VMManage{
		Event:        options.GetEvent(),
		InformerList: options.GetInformerList(),
		TTLCache:     options.GetTTLCache(),
	}
}

type VMManage struct {
	Event        *audit.Event
	EventLog     *VMEventLog
	InformerList events.InformerList
	TTLCache     events.TTLCache
}

func (m *VMManage) Log() error {
	return m.EventLog.Log()
}

func (m *VMManage) ShouldLog() bool {
	return m.EventLog.shouldLog
}

func (m *VMManage) IsMatched() bool {
	if m.Event.ObjectRef == nil || m.Event.Stage != audit.StageResponseComplete {
		return false
	}

	if m.Event.ObjectRef.Resource != "virtualmachines" {
		return false
	}

	uriWithoutQueryParams, err := util.RemoveAllQueryParams(m.Event.RequestURI)
	if err != nil {
		log.Debug("failed to remove query params from URI", err.Error(), slog.String("uri", m.Event.RequestURI))
		return false
	}

	updateURI := fmt.Sprintf("/apis/virtualization.deckhouse.io/v1alpha2/namespaces/%s/virtualmachines/%s", m.Event.ObjectRef.Namespace, m.Event.ObjectRef.Name)
	if (m.Event.Verb == "update" || m.Event.Verb == "patch" || m.Event.Verb == "delete") && updateURI == uriWithoutQueryParams {
		return true
	}

	createURI := "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/dev/virtualmachines"
	if m.Event.Verb == "create" && createURI == uriWithoutQueryParams {
		return true
	}

	return false
}

func (m *VMManage) Fill() error {
	m.EventLog = NewVMEventLog(m.Event)
	m.EventLog.Type = "Manage VM"

	switch m.Event.Verb {
	case "create":
		m.EventLog.Name = "VM creation"
	case "update", "patch":
		m.EventLog.Name = "VM update"
	case "delete":
		m.EventLog.Level = "warn"
		m.EventLog.Name = "VM deletion"
	}

	vm, err := util.GetVMFromInformer(m.TTLCache, m.InformerList.GetVMInformer(), m.Event.ObjectRef.Namespace+"/"+m.Event.ObjectRef.Name)
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
