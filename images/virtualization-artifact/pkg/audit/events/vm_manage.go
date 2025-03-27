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
	"fmt"
	"log/slog"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
)

func NewVMManage(options NewEventHandlerOptions) eventLogger {
	return &VMManage{
		event:        options.Event,
		informerList: options.InformerList,
		ttlCache:     options.TTLCache,
	}
}

type VMManage struct {
	event        *audit.Event
	eventLog     *VMEventLog
	informerList informerList
	ttlCache     ttlCache
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

	uriWithoutQueryParams, err := removeAllQueryParams(m.event.RequestURI)
	if err != nil {
		log.Error("failed to remove query params from URI", err.Error(), slog.String("uri", m.event.RequestURI))
		return false
	}

	updateURI := fmt.Sprintf("/apis/virtualization.deckhouse.io/v1alpha2/namespaces/%s/virtualmachines/%s", m.event.ObjectRef.Namespace, m.event.ObjectRef.Name)
	if (m.event.Verb == "update" || m.event.Verb == "patch" || m.event.Verb == "delete") && updateURI == uriWithoutQueryParams {
		return true
	}

	createURI := "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/dev/virtualmachines"
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
		m.eventLog.Name = "VM creation"
	case "update", "patch":
		m.eventLog.Name = "VM update"
	case "delete":
		m.eventLog.Level = "warn"
		m.eventLog.Name = "VM deletion"
	}

	vm, err := getVMFromInformer(m.ttlCache, m.informerList.GetVMInformer(), m.event.ObjectRef.Namespace+"/"+m.event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get vm from informer", log.Err(err))

		return m.eventLog.Log()
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

	return m.eventLog.Log()
}
