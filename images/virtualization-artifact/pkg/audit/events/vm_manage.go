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
		informerList: options.InformerList,
		ttlCache:     options.TTLCache,
	}
}

type VMManage struct {
	informerList informerList
	ttlCache     ttlCache
}

func (m *VMManage) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if event.ObjectRef.Resource != "virtualmachines" {
		return false
	}

	uriWithoutQueryParams, err := removeAllQueryParams(event.RequestURI)
	if err != nil {
		log.Error("failed to remove query params from URI", err.Error(), slog.String("uri", event.RequestURI))
		return false
	}

	updateURI := fmt.Sprintf("/apis/virtualization.deckhouse.io/v1alpha2/namespaces/%s/virtualmachines/%s", event.ObjectRef.Namespace, event.ObjectRef.Name)
	if (event.Verb == "update" || event.Verb == "patch" || event.Verb == "delete") && updateURI == uriWithoutQueryParams {
		return true
	}

	createURI := "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/dev/virtualmachines"
	if event.Verb == "create" && createURI == uriWithoutQueryParams {
		return true
	}

	return false
}

func (m *VMManage) Log(event *audit.Event) error {
	eventLog := NewVMEventLog(event)
	eventLog.Type = "Manage VM"

	switch event.Verb {
	case "create":
		eventLog.Name = "VM creation"
	case "update", "patch":
		eventLog.Name = "VM update"
	case "delete":
		eventLog.Level = "warn"
		eventLog.Name = "VM deletion"
	}

	vm, err := getVMFromInformer(m.ttlCache, m.informerList.GetVMInformer(), event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		log.Debug("fail to get vm from informer", log.Err(err))

		return eventLog.Log()
	}

	eventLog.VirtualmachineUID = string(vm.UID)

	if vm.Status.GuestOSInfo.Name != "" {
		eventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name
	}

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
