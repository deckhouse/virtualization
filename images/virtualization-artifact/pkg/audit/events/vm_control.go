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
	"slices"
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type NewVMControlOptions struct {
	VMInformer   cache.Indexer
	VDInformer   cache.Indexer
	NodeInformer cache.Indexer
	PodInformer  cache.Indexer
}

func NewVMControl(options NewVMControlOptions) *VMControl {
	return &VMControl{
		vmInformer:   options.VMInformer,
		vdInformer:   options.VDInformer,
		nodeInformer: options.NodeInformer,
		podInformer:  options.PodInformer,
	}
}

type VMControl struct {
	podInformer  cache.Indexer
	vmInformer   cache.Indexer
	vdInformer   cache.Indexer
	nodeInformer cache.Indexer
	vmopInformer cache.Indexer
}

func (m *VMControl) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if strings.Contains(event.ObjectRef.Name, "virt-launcher") && event.ObjectRef.Resource == "pod" && event.Verb == "delete" {
		return true
	}

	return false
}

func (m *VMControl) Log(event *audit.Event) error {
	eventLog := NewEventLog(event)
	eventLog.Type = "Control VM"

	pod, err := getPodFromInformer(m.podInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod from informer: %w", err)
	}

	terminatedStatuses := make([]string, 0, len(pod.Status.ContainerStatuses))
	for _, status := range pod.Status.ContainerStatuses {
		terminatedStatuses = append(terminatedStatuses, string(status.State.Terminated.Message))
	}

	isControllerAction := event.User.Username == "system:serviceaccount:d8-virtualization:virtualization-controller"

	if isControllerAction {
		eventLog.Level = "warn"

		if slices.Contains(terminatedStatuses, "SHUTDOWN") {
			eventLog.Name = "VM stoped from OS"
		}

		if slices.Contains(terminatedStatuses, "RESET") {
			eventLog.Name = "VM restarted from OS"
		}
	} else {
		eventLog.Level = "critical"
		eventLog.Name = "VM killed abnormal way"
	}

	vm, err := getVMFromInformer(m.vmInformer, pod.Namespace+"/"+pod.Labels["vm.kubevirt.internal.virtualization.deckhouse.io/name"])
	if err != nil {
		return fmt.Errorf("fail to get vm from informer: %w", err)
	}

	eventLog.VirtualmachineUID = string(vm.UID)
	eventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name

	if len(vm.Spec.BlockDeviceRefs) > 0 {
		if err := fillVDInfo(m.vdInformer, &eventLog, vm); err != nil {
			log.Error("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := fillNodeInfo(m.nodeInformer, &eventLog, vm); err != nil {
			log.Error("fail to fill node info", log.Err(err))
		}
	}

	return eventLog.Log()
}
