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

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func NewIntegrityCheckVM(options events.EventLoggerOptions) events.EventLogger {
	return &IntegrityCheckVM{
		Event:        options.GetEvent(),
		InformerList: options.GetInformerList(),
		TTLCache:     options.GetTTLCache(),
	}
}

type IntegrityCheckVM struct {
	Event        *audit.Event
	EventLog     *IntegrityCheckEventLog
	InformerList events.InformerList
	TTLCache     events.TTLCache
}

func (m *IntegrityCheckVM) Log() error {
	return m.EventLog.Log()
}

func (m *IntegrityCheckVM) ShouldLog() bool {
	return m.EventLog.shouldLog
}

func (m *IntegrityCheckVM) IsMatched() bool {
	if m.Event.ObjectRef == nil || m.Event.ObjectRef.Name == "" || m.Event.Stage != audit.StageResponseComplete {
		return false
	}

	if (m.Event.Verb == "patch" || m.Event.Verb == "update") && m.Event.ObjectRef.Resource == "internalvirtualizationvirtualmachineinstances" {
		return true
	}

	return false
}

func (m *IntegrityCheckVM) Fill() error {
	m.EventLog = NewIntegrityCheckEventLog(m.Event)

	m.EventLog.Name = "VM config integrity check failed"
	m.EventLog.ObjectType = "Virtual machine configuration"
	m.EventLog.ControlMethod = "Integrity Check"
	m.EventLog.ReactionType = "info"
	m.EventLog.IntegrityCheckAlgo = "sha256"

	vmi, err := util.GetInternalVMIFromInformer(m.TTLCache, m.InformerList.GetInternalVMIInformer(), m.Event.ObjectRef.Namespace+"/"+m.Event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("failed to get VMI from informer: %w", err)
	}

	if vmi.Annotations[annotations.AnnIntegrityCoreChecksum] == vmi.Annotations[annotations.AnnIntegrityCoreChecksumApplied] {
		m.EventLog.shouldLog = false
		return nil
	}

	m.EventLog.VirtualMachineName = vmi.Name
	m.EventLog.ReferenceChecksum = vmi.Annotations[annotations.AnnIntegrityCoreChecksum]
	m.EventLog.CurrentChecksum = vmi.Annotations[annotations.AnnIntegrityCoreChecksumApplied]

	return nil
}
