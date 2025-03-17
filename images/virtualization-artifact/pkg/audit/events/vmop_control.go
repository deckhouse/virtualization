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
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NewVMOPControlOptions struct {
	VMInformer   cache.Indexer
	VDInformer   cache.Indexer
	VMOPInformer cache.Indexer
	NodeInformer cache.Indexer
}

func NewVMOPControl(options NewVMOPControlOptions) *VMControl {
	return &VMControl{
		vmInformer:   options.VMInformer,
		vdInformer:   options.VDInformer,
		vmopInformer: options.VMOPInformer,
		nodeInformer: options.NodeInformer,
	}
}

type VMOPControl struct {
	vmInformer   cache.Indexer
	vdInformer   cache.Indexer
	nodeInformer cache.Indexer
	vmopInformer cache.Indexer
}

func (m *VMOPControl) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if event.ObjectRef.Resource == "virtualmachineoperations" && event.Verb == "create" {
		return true
	}

	return false
}

func (m *VMOPControl) Log(event *audit.Event) error {
	eventLog := NewEventLog(event)
	eventLog.Type = "Control VM"

	vmop, err := getVMOPFromInformer(m.vmopInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get vmop from informer: %w", err)
	}

	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		eventLog.Name = "VM started"
		eventLog.Level = "info"
	case v1alpha2.VMOPTypeStop:
		eventLog.Name = "VM stopped"
		eventLog.Level = "warn"
	case v1alpha2.VMOPTypeRestart:
		eventLog.Name = "VM restarted"
		eventLog.Level = "warn"
	case v1alpha2.VMOPTypeMigrate:
		eventLog.Name = "VM migrated"
		eventLog.Level = "warn"
	case v1alpha2.VMOPTypeEvict:
		eventLog.Name = "VM evicted"
		eventLog.Level = "warn"
	}

	vm, err := getVMFromInformer(m.vmInformer, vmop.Namespace+"/"+vmop.Spec.VirtualMachine)
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
