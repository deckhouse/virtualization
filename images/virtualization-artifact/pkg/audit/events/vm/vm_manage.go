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
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
)

func NewVMManage(options events.EventLoggerOptions) *VMManage {
	return &VMManage{
		event:        options.GetEvent(),
		informerList: options.GetInformerList(),
		ttlCache:     options.GetTTLCache(),
	}
}

type VMManage struct {
	event        *audit.Event
	eventLog     *VMEventLog
	informerList events.InformerList
	ttlCache     events.TTLCache
}

func (m *VMManage) Log() error {
	return m.eventLog.Log()
}

func (m *VMManage) ShouldLog() bool {
	return m.eventLog.shouldLog
}

func (m *VMManage) IsMatched() bool {
	if m.event.ObjectRef == nil || m.event.Stage != audit.StageResponseComplete {
		return false
	}

	if m.event.ObjectRef.Resource != "virtualmachines" {
		return false
	}

	if strings.HasPrefix(m.event.User.Username, "system:") &&
		!strings.HasPrefix(m.event.User.Username, "system:serviceaccount:d8-service-accounts") {
		return false
	}

	uriWithoutQueryParams, err := util.RemoveAllQueryParams(m.event.RequestURI)
	if err != nil {
		log.Debug("failed to remove query params from URI", err.Error(), slog.String("uri", m.event.RequestURI))
		return false
	}

	updateURI := fmt.Sprintf("/apis/virtualization.deckhouse.io/v1alpha2/namespaces/%s/virtualmachines/%s", m.event.ObjectRef.Namespace, m.event.ObjectRef.Name)
	if (m.event.Verb == "update" || m.event.Verb == "patch" || m.event.Verb == "delete") && updateURI == uriWithoutQueryParams {
		return true
	}

	createURI := fmt.Sprintf("/apis/virtualization.deckhouse.io/v1alpha2/namespaces/%s/virtualmachines", m.event.ObjectRef.Namespace)
	if m.event.Verb == "create" && createURI == uriWithoutQueryParams {
		return true
	}

	return false
}

func (m *VMManage) Fill() error {
	m.eventLog = NewVMEventLog(m.event)
	m.eventLog.Type = "Manage VM"

	switch m.event.Verb {
	case "create":
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been created by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
	case "update", "patch":
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been updated by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
	case "delete":
		m.eventLog.Level = "warn"
		m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' has been deleted by '%s'", m.event.ObjectRef.Name, m.event.User.Username)
	}

	vm, err := util.GetVMFromInformer(m.ttlCache, m.informerList.GetVMInformer(), m.event.ObjectRef.Namespace+"/"+m.event.ObjectRef.Name)
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
