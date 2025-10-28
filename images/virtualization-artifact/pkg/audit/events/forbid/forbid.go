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

package forbid

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func NewForbid(options events.EventLoggerOptions) *Forbid {
	return &Forbid{
		ctx:      options.GetCtx(),
		event:    options.GetEvent(),
		client:   options.GetClient(),
		ttlCache: options.GetTTLCache(),
	}
}

type Forbid struct {
	event    *audit.Event
	eventLog *ForbidEventLog
	ctx      context.Context
	ttlCache events.TTLCache
	client   kubernetes.Interface
}

func (m *Forbid) Log() error {
	return m.eventLog.Log()
}

func (m *Forbid) ShouldLog() bool {
	return m.eventLog.shouldLog
}

func (m *Forbid) IsMatched() bool {
	if m.event.ObjectRef == nil || m.event.Stage != audit.StageResponseComplete {
		return false
	}

	if strings.HasPrefix(m.event.User.Username, "system:") &&
		!strings.HasPrefix(m.event.User.Username, "system:serviceaccount:d8-service-accounts") {
		return false
	}

	if m.event.Annotations[annotations.AnnAuditDecision] == "forbid" {
		return true
	}

	return false
}

func (m *Forbid) Fill() error {
	m.eventLog = NewForbidEventLog(m.event)
	m.eventLog.Type = "Forbidden operation"

	m.eventLog.SourceIP = strings.Join(m.event.SourceIPs, ",")

	resource := m.event.ObjectRef.Resource
	if m.event.ObjectRef.Namespace != "" {
		resource += "/" + m.event.ObjectRef.Namespace
	}
	if m.event.ObjectRef.Name != "" {
		resource += "/" + m.event.ObjectRef.Name
	}

	m.eventLog.Name = fmt.Sprintf(
		"User '%s' attempted to perform a forbidden operation '%s' on resource '%s'.",
		m.event.User.Username,
		m.event.Verb,
		resource)

	isAdmin, err := m.isAdmin(m.event.User.Username)
	if err != nil {
		log.Debug(err.Error())
	}

	m.eventLog.IsAdmin = isAdmin

	return nil
}

func (m *Forbid) isAdmin(user string) (bool, error) {
	isAdm, ok := m.ttlCache.Get("is_admin/" + user)
	if ok {
		return isAdm.(bool), nil
	}

	canUpdateModuleConfigs, err := util.CheckAccess(m.ctx, m.client, user, "update", "authorization.k8s.io", "v1", "moduleconfigs")
	if err != nil {
		return false, err
	}

	if canUpdateModuleConfigs {
		return true, nil
	}

	canUpdateVMClasses, err := util.CheckAccess(m.ctx, m.client, user, "update", "authorization.k8s.io", "v1", "virtualmachineclasses")
	if err != nil {
		return false, err
	}

	if canUpdateVMClasses {
		return true, nil
	}

	return false, nil
}
