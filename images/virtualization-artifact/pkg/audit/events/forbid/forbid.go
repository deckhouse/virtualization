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
		Ctx:      options.GetCtx(),
		Event:    options.GetEvent(),
		Client:   options.GetClient(),
		TTLCache: options.GetTTLCache(),
	}
}

type Forbid struct {
	Event    *audit.Event
	EventLog *ForbidEventLog
	Ctx      context.Context
	TTLCache events.TTLCache
	Client   kubernetes.Interface
}

func (m *Forbid) Log() error {
	return m.EventLog.Log()
}

func (m *Forbid) ShouldLog() bool {
	return m.EventLog.shouldLog
}

func (m *Forbid) IsMatched() bool {
	if m.Event.ObjectRef == nil || m.Event.Stage != audit.StageResponseComplete {
		return false
	}

	if m.Event.Annotations[annotations.AnnAuditDecision] == "forbid" {
		return true
	}

	return false
}

func (m *Forbid) Fill() error {
	m.EventLog = NewForbidEventLog(m.Event)
	m.EventLog.Type = "Forbidden operation"

	m.EventLog.SourceIP = strings.Join(m.Event.SourceIPs, ",")

	resource := m.Event.ObjectRef.Resource
	if m.Event.ObjectRef.Namespace != "" {
		resource += "/" + m.Event.ObjectRef.Namespace
	}
	if m.Event.ObjectRef.Name != "" {
		resource += "/" + m.Event.ObjectRef.Name
	}

	m.EventLog.Name = fmt.Sprintf(
		"User (%s) attempted to perform a forbidden operation (%s) on resource (%s).",
		m.Event.User.Username,
		m.Event.Verb,
		resource)

	isAdmin, err := m.isAdmin(m.Event.User.Username)
	if err != nil {
		log.Debug(err.Error())
	}

	m.EventLog.IsAdmin = isAdmin

	return nil
}

func (m *Forbid) isAdmin(user string) (bool, error) {
	isAdm, ok := m.TTLCache.Get("is_admin/" + user)
	if ok {
		return isAdm.(bool), nil
	}

	canUpdateModuleConfigs, err := util.CheckAccess(m.Ctx, m.Client, user, "update", "authorization.k8s.io", "v1", "moduleconfigs")
	if err != nil {
		return false, err
	}

	if canUpdateModuleConfigs {
		return true, nil
	}

	canUpdateVMClasses, err := util.CheckAccess(m.Ctx, m.Client, user, "update", "authorization.k8s.io", "v1", "virtualmachineclasses")
	if err != nil {
		return false, err
	}

	if canUpdateVMClasses {
		return true, nil
	}

	return false, nil
}
