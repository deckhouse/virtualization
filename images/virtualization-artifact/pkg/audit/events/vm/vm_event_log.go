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

package vm

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMEventLog struct {
	Type           string `json:"type"`
	Level          string `json:"level"`
	Name           string `json:"name"`
	Datetime       string `json:"datetime"`
	Uid            string `json:"uid"`
	RequestSubject string `json:"request_subject"`

	ActionType         string `json:"action_type"`
	NodeNetworkAddress string `json:"node_network_address"`
	VirtualmachineUID  string `json:"virtualmachine_uid"`
	VirtualmachineOS   string `json:"virtualmachine_os"`
	StorageClasses     string `json:"storageclasses"`
	FirmwareVersion    string `json:"firmware_version"`

	OperationResult string `json:"operation_result"`

	shouldLog bool
}

func NewVMEventLog(event *audit.Event) *VMEventLog {
	eventLog := &VMEventLog{
		Type:            "unknown",
		Level:           "info",
		Name:            "unknown",
		Datetime:        event.RequestReceivedTimestamp.Format(time.RFC3339),
		Uid:             string(event.AuditID),
		RequestSubject:  event.User.Username,
		OperationResult: "unknown",

		ActionType:         event.Verb,
		NodeNetworkAddress: "unknown",
		VirtualmachineUID:  "unknown",
		VirtualmachineOS:   "unknown",
		StorageClasses:     "unknown",
		FirmwareVersion:    "unknown",

		shouldLog: true,
	}

	if event.Annotations[annotations.AnnAuditDecision] != "" {
		eventLog.OperationResult = event.Annotations[annotations.AnnAuditDecision]
	}

	return eventLog
}

func (e VMEventLog) Log() error {
	bytes, err := json.Marshal(e)
	if err != nil {
		return err
	}

	fmt.Println(string(bytes))

	return nil
}

func (e *VMEventLog) fillVDInfo(ttlCache events.TTLCache, vdInformer events.Indexer, vm *v1alpha2.VirtualMachine) error {
	storageClasses := []string{}

	for _, bd := range vm.Spec.BlockDeviceRefs {
		if bd.Kind != v1alpha2.VirtualDiskKind {
			continue
		}

		vd, err := util.GetVDFromInformer(ttlCache, vdInformer, vm.Namespace+"/"+bd.Name)
		if err != nil {
			return fmt.Errorf("fail to get virtual disk from informer: %w", err)
		}

		storageClasses = append(storageClasses, vd.Status.StorageClassName)
	}

	if len(storageClasses) != 0 {
		e.StorageClasses = strings.Join(slices.Compact(storageClasses), ",")
	}

	return nil
}

func (e *VMEventLog) fillNodeInfo(nodeInformer events.Indexer, vm *v1alpha2.VirtualMachine) error {
	node, err := util.GetNodeFromInformer(nodeInformer, vm.Status.Node)
	if err != nil {
		return fmt.Errorf("fail to get node from informer: %w", err)
	}

	addresses := []string{}
	for _, addr := range node.Status.Addresses {
		if addr.Type != corev1.NodeHostName && addr.Address != "" {
			addresses = append(addresses, addr.Address)
		}
	}

	if len(addresses) != 0 {
		e.NodeNetworkAddress = strings.Join(slices.Compact(addresses), ",")
	}

	return nil
}
