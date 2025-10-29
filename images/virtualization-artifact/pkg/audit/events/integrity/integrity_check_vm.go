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

package integrity

import (
	"fmt"
	"strings"

	authnv1 "k8s.io/api/authentication/v1"
	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func NewIntegrityCheckVM(options events.EventLoggerOptions) *IntegrityCheckVM {
	return &IntegrityCheckVM{
		event:        options.GetEvent(),
		informerList: options.GetInformerList(),
		ttlCache:     options.GetTTLCache(),
	}
}

type IntegrityCheckVM struct {
	event        *audit.Event
	eventLog     *IntegrityCheckEventLog
	informerList events.InformerList
	ttlCache     events.TTLCache
}

func (m *IntegrityCheckVM) Log() error {
	return m.eventLog.Log()
}

func (m *IntegrityCheckVM) ShouldLog() bool {
	return m.eventLog.shouldLog
}

func (m *IntegrityCheckVM) IsMatched() bool {
	if m.event.ObjectRef == nil || m.event.ObjectRef.Name == "" || m.event.Stage != audit.StageResponseComplete {
		return false
	}

	if m.ignoreForSystemUsers(m.event.User) {
		return false
	}

	if (m.event.Verb == "patch" || m.event.Verb == "update") && m.event.ObjectRef.Resource == "internalvirtualizationvirtualmachineinstances" {
		return true
	}

	return false
}

func (m *IntegrityCheckVM) Fill() error {
	m.eventLog = NewIntegrityCheckEventLog(m.event)

	vmi, err := util.GetInternalVMIFromInformer(m.ttlCache, m.informerList.GetInternalVMIInformer(), m.event.ObjectRef.Namespace+"/"+m.event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("failed to get VMI from informer: %w", err)
	}

	m.eventLog.Name = fmt.Sprintf("Virtual machine '%s' config integrity check failed", vmi.Name)
	m.eventLog.ObjectType = "Virtual machine configuration"
	m.eventLog.ControlMethod = "Integrity Check"
	m.eventLog.ReactionType = "info"
	m.eventLog.IntegrityCheckAlgo = "sha256"

	m.eventLog.VirtualMachineName = vmi.Name
	m.eventLog.ReferenceChecksum = vmi.Annotations[annotations.AnnIntegrityCoreChecksum]
	m.eventLog.CurrentChecksum = vmi.Annotations[annotations.AnnIntegrityCoreChecksumApplied]

	if vmi.Annotations[annotations.AnnIntegrityCoreChecksum] == vmi.Annotations[annotations.AnnIntegrityCoreChecksumApplied] {
		m.eventLog.shouldLog = false
		return nil
	}

	return nil
}

func (m *IntegrityCheckVM) ignoreForSystemUsers(userInfo authnv1.UserInfo) bool {
	// Do not ignore for d8 service accounts.
	if strings.HasPrefix(userInfo.Username, "system:serviceaccount:d8-service-accounts") {
		return false
	}
	// Do not ignore for virtualization controller.
	if strings.HasPrefix(userInfo.Username, "system:serviceaccount:d8-virtualization") {
		return false
	}
	// Ignore for all other system users, not ignore for non-system users.
	return strings.HasPrefix(m.event.User.Username, "system:")
}
