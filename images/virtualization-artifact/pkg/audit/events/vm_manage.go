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

type NewVMManageOptions struct {
	VMInformer   cache.Indexer
	VDInformer   cache.Indexer
	NodeInformer cache.Indexer
}

func NewVMManage(options NewVMManageOptions) *VMManage {
	return &VMManage{
		vmInformer:   options.VMInformer,
		nodeInformer: options.NodeInformer,
		vdInformer:   options.VDInformer,
	}
}

type VMManage struct {
	vmInformer   cache.Indexer
	vdInformer   cache.Indexer
	nodeInformer cache.Indexer
}

func (m *VMManage) IsMatched(event *audit.Event) bool {
	if event.ObjectRef.Resource != "virtualmachines" || event.Stage != audit.StageResponseComplete {
		return false
	}

	uri := fmt.Sprintf("/apis/virtualization.deckhouse.io/v1alpha2/namespaces/%s/virtualmachines/%s", event.ObjectRef.Namespace, event.ObjectRef.Name)
	if (event.Verb == "update" || event.Verb == "patch" || event.Verb == "delete") && uri == event.RequestURI {
		return true
	}

	uriWithoutQueryParams, err := removeAllQueryParams(event.RequestURI)
	if err != nil {
		log.Error("failed to remove query params from URI", err.Error(), slog.String("uri", event.RequestURI))
		return false
	}

	createURI := "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/dev/virtualmachines"
	if event.Verb == "create" && createURI == uriWithoutQueryParams {
		return true
	}

	return false
}

func (m *VMManage) Log(event *audit.Event) error {
	response := map[string]string{
		"type":           "Manage VM",
		"level":          string(event.Level),
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

	switch event.Verb {
	case "create":
		response["name"] = "VM creation"
	case "update", "patch":
		response["name"] = "VM update"
	case "delete":
		response["level"] = "warn"
		response["name"] = "VM deletion"
	}

	vm, err := getVMFromInformer(m.vmInformer, event)
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
	log.Info("VirtualMachine", logSlice...)

	return nil
}
