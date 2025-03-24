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
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

type ModuleEventLog struct {
	Type            string `json:"type"`
	Level           string `json:"level"`
	Name            string `json:"name"`
	Datetime        string `json:"datetime"`
	Uid             string `json:"uid"`
	RequestSubject  string `json:"request_subject"`
	OperationResult string `json:"operation_result"`

	ActionType            string `json:"action_type"`
	Component             string `json:"component"`
	NodeNetworkAddress    string `json:"node_network_address"`
	VirtualizationVersion string `json:"virtualization_version"`
	VirtualizationName    string `json:"virtualization_name"`
	FirmwareVersion       string `json:"firmware_version"`
}

func NewModuleEventLog(event *audit.Event) ModuleEventLog {
	eventLog := ModuleEventLog{
		Type:            "unknown",
		Level:           "info",
		Name:            "unknown",
		Datetime:        event.RequestReceivedTimestamp.Format(time.RFC3339),
		Uid:             string(event.AuditID),
		RequestSubject:  event.User.Username,
		OperationResult: "unknown",

		ActionType:            event.Verb,
		Component:             "virtualizaion",
		NodeNetworkAddress:    "unknown",
		VirtualizationName:    "Deckhouse Virtualization Platform",
		VirtualizationVersion: "unknown",
		FirmwareVersion:       "unknown",
	}

	if event.Annotations[annotations.AnnAuditDecision] != "" {
		eventLog.OperationResult = event.Annotations[annotations.AnnAuditDecision]
	}

	return eventLog
}

func (e ModuleEventLog) Log() error {
	bytes, err := json.Marshal(e)
	if err != nil {
		return err
	}

	fmt.Println(string(bytes))

	return nil
}

func (e *ModuleEventLog) fillNodeInfo(nodeInformer indexer, pod *corev1.Pod) error {
	node, err := getNodeFromInformer(nodeInformer, pod.Spec.NodeName)
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
