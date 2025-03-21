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

type NewVMControlOptions struct {
	VMInformer   indexer
	VDInformer   indexer
	NodeInformer indexer
	PodInformer  indexer
	TTLCache     ttlCache
}

func NewVMControl(options NewVMControlOptions) *VMControl {
	return &VMControl{
		vmInformer:   options.VMInformer,
		vdInformer:   options.VDInformer,
		nodeInformer: options.NodeInformer,
		podInformer:  options.PodInformer,
		ttlCache:     options.TTLCache,
	}
}

type VMControl struct {
	podInformer  indexer
	vmInformer   indexer
	vdInformer   indexer
	nodeInformer indexer
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

	pod, err := getPodFromInformer(m.ttlCache, m.podInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
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

	if isControllerAction {
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
	} else if isNodeAction {
		return nil
	} else {
		eventLog.Level = "critical"
		eventLog.Name = "VM killed abnormal way"
	}

	vm, err := getVMFromInformer(m.ttlCache, m.vmInformer, pod.Namespace+"/"+pod.Labels["vm.kubevirt.internal.virtualization.deckhouse.io/name"])
	if err != nil {
		return fmt.Errorf("fail to get vm from informer: %w", err)
	}

	eventLog.VirtualmachineUID = string(vm.UID)
	eventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name

	if len(vm.Spec.BlockDeviceRefs) > 0 {
		if err := eventLog.fillVDInfo(m.ttlCache, m.vdInformer, vm); err != nil {
			log.Debug("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := eventLog.fillNodeInfo(m.nodeInformer, vm); err != nil {
			log.Debug("fail to fill node info", log.Err(err))
		}
	}

	return eventLog.Log()
}
