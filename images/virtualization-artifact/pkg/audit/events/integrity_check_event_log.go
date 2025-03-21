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
	"time"

	"k8s.io/apiserver/pkg/apis/audit"
)

type IntegrityCheckEventLog struct {
	Type            string `json:"type"`
	Level           string `json:"level"`
	Name            string `json:"name"`
	Datetime        string `json:"datetime"`
	Uid             string `json:"uid"`
	RequestSubject  string `json:"request_subject"`
	OperationResult string `json:"operation_result"`

	ObjectType         string `json:"object_type"`
	VirtualMachineName string `json:"virtual_machine_name"`
	ControlMethod      string `json:"control_method"`
	ReactionType       string `json:"reaction_type"`
	IntegrityCheckAlgo string `json:"integrity_check_algo"`
	ReferenceChecksum  string `json:"reference_checksum"`
	CurrentChecksum    string `json:"current_checksum"`
}

func NewIntegrityCheckEventLog(event *audit.Event) IntegrityCheckEventLog {
	eventLog := IntegrityCheckEventLog{
		Type:            "unknown",
		Level:           "info",
		Name:            "unknown",
		Datetime:        event.RequestReceivedTimestamp.Format(time.RFC3339),
		Uid:             string(event.AuditID),
		RequestSubject:  event.User.Username,
		OperationResult: "unknown",

		ObjectType:         "unknown",
		VirtualMachineName: "unknown",
		ControlMethod:      "unknown",
		ReactionType:       "unknown",
		IntegrityCheckAlgo: "unknown",
		ReferenceChecksum:  "unknown",
		CurrentChecksum:    "unknown",
	}

	if event.Annotations["authorization.k8s.io/decision"] != "" {
		eventLog.OperationResult = event.Annotations["authorization.k8s.io/decision"]
	}

	return eventLog
}

func (e *IntegrityCheckEventLog) Log() error {
	bytes, err := json.Marshal(e)
	if err != nil {
		return err
	}

	fmt.Println(string(bytes))

	return nil
}
