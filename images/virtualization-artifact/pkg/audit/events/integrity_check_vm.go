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

	"k8s.io/apiserver/pkg/apis/audit"
)

type NewIntegrityCheckVMOptions struct {
	InternalVMIInformer indexer
	VMInformer          indexer
	TTLCache            ttlCache
}

func NewIntegrityCheckVM(options NewIntegrityCheckVMOptions) *IntegrityCheckVM {
	return &IntegrityCheckVM{
		internalVMIInformer: options.InternalVMIInformer,
		vmInformer:          options.VMInformer,
		ttlCache:            options.TTLCache,
	}
}

type IntegrityCheckVM struct {
	internalVMIInformer indexer
	vmInformer          indexer
	ttlCache            ttlCache
}

func (m *IntegrityCheckVM) IsMatched(event *audit.Event) bool {
	if (event.ObjectRef == nil && event.ObjectRef.Name != "") || event.Stage != audit.StageResponseComplete {
		return false
	}

	if (event.Verb == "patch" || event.Verb == "update") && event.ObjectRef.Resource == "internalvirtualizationvirtualmachineinstances" {
		return true
	}

	return false
}

func (m *IntegrityCheckVM) Log(event *audit.Event) error {
	eventLog := NewIntegrityCheckEventLog(event)
	eventLog.Type = "Integrity Check"

	vmi, err := getInternalVMIFromInformer(m.internalVMIInformer, event.ObjectRef.Namespace+"/"+event.ObjectRef.Name)
	if err != nil {
		return fmt.Errorf("failed to get VMI from informer: %w", err)
	}

	eventLog.VirtualMachineName = vmi.Name

	return eventLog.Log()
}
