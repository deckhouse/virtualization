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
	"context"
	"fmt"
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func NewForbid(options NewEventHandlerOptions) eventLogger {
	return &Forbid{
		ctx:       options.Ctx,
		clientset: options.Client,
		ttlCache:  options.TTLCache,
	}
}

type Forbid struct {
	ctx       context.Context
	ttlCache  ttlCache
	clientset *kubernetes.Clientset
	eventLog  *ForbidEventLog
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
	m.eventLog = NewForbidEventLog(event)
	m.eventLog.Type = "Forbidden operation"

	m.eventLog.SourceIP = strings.Join(event.SourceIPs, ",")

	resource := event.ObjectRef.Resource
	if event.ObjectRef.Namespace != "" {
		resource += "/" + event.ObjectRef.Namespace
	}
	if event.ObjectRef.Name != "" {
		resource += "/" + event.ObjectRef.Name
	}

	m.eventLog.Name = fmt.Sprintf(
		"User (%s) attempted to perform a forbidden operation (%s) on resource (%s).",
		event.User.Username,
		event.Verb,
		resource)

	isAdmin, err := m.isAdmin(event.User.Username)
	if err != nil {
		log.Debug(err.Error())
	}

	m.eventLog.IsAdmin = isAdmin

	return m.eventLog
}

func (m *Forbid) isAdmin(user string) (bool, error) {
	isAdm, ok := m.ttlCache.Get("is_admin/" + user)
	if ok {
		return isAdm.(bool), nil
	}

	canUpdateModuleConfigs, err := checkAccess(m.ctx, m.clientset, user, "update", "authorization.k8s.io", "v1", "moduleconfigs")
	if err != nil {
		return false, err
	}

	if canUpdateModuleConfigs {
		return true, nil
	}

	canUpdateVMClasses, err := checkAccess(m.ctx, m.clientset, user, "update", "authorization.k8s.io", "v1", "virtualmachineclasses")
	if err != nil {
		return false, err
	}

	if canUpdateVMClasses {
		return true, nil
	}

	return false, nil
}
