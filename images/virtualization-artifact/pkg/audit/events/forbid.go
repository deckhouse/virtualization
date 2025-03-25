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
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

type NewForbidOptions struct {
	AdminInformer indexer
	TTLCache      ttlCache
}

func NewForbid(options NewForbidOptions) *Forbid {
	return &Forbid{
		adminInformer: options.AdminInformer,
		ttlCache:      options.TTLCache,
	}
}

type Forbid struct {
	adminInformer indexer
	ttlCache      ttlCache
}

func (m *Forbid) IsMatched(event *audit.Event) bool {
	if event.ObjectRef == nil || event.Stage != audit.StageResponseComplete {
		return false
	}

	if event.Annotations[annotations.AnnAuditDecision] == "forbid" {
		return true
	}

	return false
}

func (m *Forbid) Log(event *audit.Event) error {
	eventLog := NewForbidEventLog(event)
	eventLog.Type = "Forbidden operation"

	eventLog.SourceIP = strings.Join(event.SourceIPs, ",")

	resource := fmt.Sprintf("%s/%s", event.ObjectRef.Resource, event.ObjectRef.Namespace)
	if event.ObjectRef.Name != "" {
		resource = resource + "/" + event.ObjectRef.Name
	}

	eventLog.Name = fmt.Sprintf(
		"User (%s) attempted to perform a forbidden operation (%s) on resource (%s).",
		event.User.Username,
		event.Verb,
		resource)

	return eventLog.Log()
}
