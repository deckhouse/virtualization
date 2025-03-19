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

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type NewVMConnectOptions struct {
	VMInformer   indexer
	VDInformer   indexer
	NodeInformer indexer
}

func NewVMConnect(options NewVMConnectOptions) *VMAccess {
	return &VMAccess{
		vmInformer:   options.VMInformer,
		nodeInformer: options.NodeInformer,
		vdInformer:   options.VDInformer,
	}
}

type VMAccess struct {
	vmInformer   indexer
	vdInformer   indexer
	nodeInformer indexer
}

func (m *VMAccess) IsMatched(event *audit.Event) bool {
	if event.Stage != audit.StageResponseComplete || event.ObjectRef == nil {
		return false
	}

	if event.ObjectRef.Resource != "virtualmachines" || event.ObjectRef.APIGroup != "subresources.virtualization.deckhouse.io" {
		return false
	}

	if event.ObjectRef.Subresource == "console" || event.ObjectRef.Subresource == "vnc" || event.ObjectRef.Subresource == "portforward" {
		return true
	}

	return false
}

func (m *VMAccess) Log(event *audit.Event) error {
	eventLog := NewVMEventLog(event)
	eventLog.Type = "Access to VM"

	switch event.ObjectRef.Subresource {
	case "console":
		eventLog.Name = "Access to VM via serial console"
	case "vnc":
		eventLog.Name = "Access to VM via VNC"
	case "portforward":
		eventLog.Name = "Access to VM via portforward"
	}

	vm, err := getVMFromInformer(m.vmInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get vm from informer: %w", err)
	}

	eventLog.VirtualmachineUID = string(vm.UID)
	eventLog.VirtualmachineOS = vm.Status.GuestOSInfo.Name

	if len(vm.Spec.BlockDeviceRefs) > 0 {
		if err := eventLog.fillVDInfo(m.vdInformer, vm); err != nil {
			log.Error("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := eventLog.fillNodeInfo(m.nodeInformer, vm); err != nil {
			log.Error("fail to fill node info", log.Err(err))
		}
	}

	return eventLog.Log()
}
