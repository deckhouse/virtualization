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
)

type NewVMConnectOptions struct {
	VMInformer   cache.Indexer
	VDInformer   cache.Indexer
	NodeInformer cache.Indexer
}

func NewVMConnect(options NewVMConnectOptions) *VMAccess {
	return &VMAccess{
		vmInformer:   options.VMInformer,
		nodeInformer: options.NodeInformer,
		vdInformer:   options.VDInformer,
	}
}

type VMAccess struct {
	vmInformer   cache.Indexer
	vdInformer   cache.Indexer
	nodeInformer cache.Indexer
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
	response := map[string]string{
		"type":           "Access to VM",
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

		"operation-result": event.Annotations["authorization.k8s.io/decision"],
	}

	switch event.ObjectRef.Subresource {
	case "console":
		response["name"] = "Access to VM via serial console"
	case "vnc":
		response["name"] = "Access to VM via VNC"
	case "portforward":
		response["name"] = "Access to VM via portforward"
	}

	vm, err := getVMFromInformer(m.vmInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
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

	logSlice := []any{}
	for k, v := range response {
		logSlice = append(logSlice, slog.String(k, v))
	}
	log.Info("VMConnect", logSlice...)

	return nil
}
