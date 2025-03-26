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
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
)

func NewVMControl(options NewEventHandlerOptions) eventLogger {
	return &VMControl{
		informerList: options.InformerList,
		ttlCache:     options.TTLCache,
	}
}

type VMControl struct {
	informerList informerList
	ttlCache     ttlCache
}

func (m *VMControl) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if strings.Contains(event.ObjectRef.Name, "virt-launcher") && event.ObjectRef.Resource == "pods" && event.Verb == "delete" {
		return true
	}

	return false
}

func (m *VMControl) Log(event *audit.Event) error {
	eventLog := NewVMEventLog(event)
	eventLog.Type = "Control VM"

	pod, err := getPodFromInformer(m.ttlCache, m.informerList.GetPodInformer(), event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod from informer: %w", err)
	}

	var terminatedStatuses string
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "compute" && status.State.Terminated != nil {
			terminatedStatuses = status.State.Terminated.Message
		}
	}

	isControllerAction := strings.Contains(event.User.Username, "system:serviceaccount:d8-virtualization")
	isNodeAction := strings.Contains(event.User.Username, "system:node")

	switch {
	case isControllerAction:
		eventLog.Level = "warn"

		switch {
		case strings.Contains(terminatedStatuses, "guest-shutdown"):
			eventLog.Name = "VM stoped from OS"
		case strings.Contains(terminatedStatuses, "guest-reset"):
			eventLog.Name = "VM restarted from OS"
		default:
			// deleted by vmop
			return nil
		}
	case isNodeAction:
		return nil
	default:
		eventLog.Level = "critical"
		eventLog.Name = "VM killed abnormal way"
	}

	vm, err := getVMFromInformer(m.ttlCache, m.informerList.GetVMInformer(), pod.Namespace+"/"+pod.Labels["vm.kubevirt.internal.virtualization.deckhouse.io/name"])
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
