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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

type ForbidEventLog struct {
	Type            string `json:"type"`
	Level           string `json:"level"`
	Name            string `json:"name"`
	Datetime        string `json:"datetime"`
	Uid             string `json:"uid"`
	RequestSubject  string `json:"request_subject"`
	OperationResult string `json:"operation_result"`
	IsAdmin         string `json:"is_admin"`
	SourceIP        string `json:"source_ip"`
	ForbidReason    string `json:"forbid_reason"`
}

func NewForbidEventLog(event *audit.Event) ForbidEventLog {
	eventLog := ForbidEventLog{
		Type:            "unknown",
		Level:           "warn",
		Name:            "unknown",
		Datetime:        event.RequestReceivedTimestamp.Format(time.RFC3339),
		Uid:             string(event.AuditID),
		RequestSubject:  event.User.Username,
		OperationResult: "forbid",
		ForbidReason:    "unknown",
	}

	if event.Annotations[annotations.AnnAuditDecision] != "" {
		eventLog.OperationResult = event.Annotations[annotations.AnnAuditDecision]
	}

	if event.Annotations[annotations.AnnAuditReason] != "" {
		eventLog.OperationResult = event.Annotations[annotations.AnnAuditReason]
	}

	return eventLog
}

func (e *ForbidEventLog) Log() error {
	bytes, err := json.Marshal(e)
	if err != nil {
		return err
	}

	fmt.Println(string(bytes))

	return nil
}
