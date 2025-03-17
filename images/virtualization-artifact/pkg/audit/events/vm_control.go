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
	"time"

	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NewVMControlOptions struct {
	VMInformer   cache.Indexer
	VDInformer   cache.Indexer
	VMOPInformer cache.Indexer
	NodeInformer cache.Indexer
}

func NewVMControl(options NewVMControlOptions) *VMControl {
	return &VMControl{
		vmInformer:   options.VMInformer,
		nodeInformer: options.NodeInformer,
		vdInformer:   options.VDInformer,
		vmopInformer: options.VMOPInformer,
	}
}

type VMControl struct {
	vmInformer   cache.Indexer
	vdInformer   cache.Indexer
	nodeInformer cache.Indexer
	vmopInformer cache.Indexer
}

func (m *VMControl) IsMatched(event *audit.Event) bool {
	if event.ObjectRef.Resource != "virtualmachines" || event.Stage != audit.StageResponseComplete {
		return false
	}

	if event.ObjectRef.Resource == "virtualmachineoperations" {
		return true
	}

	return false
}

func (m *VMControl) Log(event *audit.Event) error {
	response := map[string]string{
		"type":           "Control VM",
		"level":          "info",
		"name":           "unknown",
		"datetime":       event.RequestReceivedTimestamp.Format(time.RFC3339),
		"uid":            string(event.AuditID),
		"requestSubject": event.User.Username,

		"action-type":          event.Verb,
		"node-network-address": "unknown",
		"virtualmachine-uid":   "unknown",
		"virtualmachine-os":    "unknown",
		"storageclasses":       "unknown",
		"qemu-version":         "unknown",
		"libvirt-version":      "unknown",
		"libvirt-uri":          "unknown",

		"operation-result": event.Annotations["authorization.k8s.io/decision"],
	}

	vmop, err := getVMOPFromInformer(m.vmopInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("fail to get vmop from informer: %w", err)
	}

	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		response["name"] = "VM started"
		response["level"] = "info"
	case v1alpha2.VMOPTypeStop:
		response["name"] = "VM stopped"
		response["level"] = "warn"
	case v1alpha2.VMOPTypeRestart:
		response["name"] = "VM restarted"
		response["level"] = "warn"
	case v1alpha2.VMOPTypeMigrate:
		response["name"] = "VM migrated"
		response["level"] = "warn"
	case v1alpha2.VMOPTypeEvict:
		response["name"] = "VM evicted"
		response["level"] = "warn"
	}

	vm, err := getVMFromInformer(m.vmInformer, vmop.Namespace+"/"+vmop.Spec.VirtualMachine)
	if err != nil {
		return fmt.Errorf("fail to get vm from informer: %w", err)
	}

	if len(vm.Spec.BlockDeviceRefs) > 0 {
		if err := fillVDInfo(m.vdInformer, response, vm); err != nil {
			log.Error("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := fillNodeInfo(m.nodeInformer, response, vm); err != nil {
			log.Error("fail to fill node info", log.Err(err))
		}
	}

	response["virtualmachine-uid"] = string(vm.UID)
	response["virtualmachine-os"] = vm.Status.GuestOSInfo.Name

	logSlice := make([]any, 0, len(response))
	for k, v := range response {
		logSlice = append(logSlice, slog.String(k, v))
	}
	log.Info("VirtualMachine", logSlice...)

	return nil
}
