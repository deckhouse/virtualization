/*
Copyright 2024 Flant JSC

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

package kvbuilder

import (
	"encoding/json"
	"fmt"
	"strings"

	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// LoadLastAppliedSpec loads VM spec from JSON in the last-applied-spec annotation.
func LoadLastAppliedSpec(kvvm *virtv1.VirtualMachine) (*v1alpha2.VirtualMachineSpec, error) {
	lastSpecJSON := kvvm.GetAnnotations()[annotations.AnnVMLastAppliedSpec]
	if strings.TrimSpace(lastSpecJSON) == "" {
		return nil, nil
	}

	var spec v1alpha2.VirtualMachineSpec
	err := json.Unmarshal([]byte(lastSpecJSON), &spec)
	if err != nil {
		return nil, fmt.Errorf("load spec from JSON: %w", err)
	}
	return &spec, nil
}

// SetLastAppliedSpec updates the last-applied-spec annotation with VM spec JSON.
func SetLastAppliedSpec(kvvm *virtv1.VirtualMachine, vm *v1alpha2.VirtualMachine) error {
	lastApplied, err := json.Marshal(vm.Spec)
	if err != nil {
		return fmt.Errorf("convert spec to JSON: %w", err)
	}

	annotations.AddAnnotation(kvvm, annotations.AnnVMLastAppliedSpec, string(lastApplied))
	return nil
}

// LoadLastAppliedClassSpec loads VMClass spec from JSON in the last-applied-spec annotation.
func LoadLastAppliedClassSpec(kvvm *virtv1.VirtualMachine) (*v1alpha2.VirtualMachineClassSpec, error) {
	lastSpecJSON := kvvm.GetAnnotations()[annotations.AnnVMClassLastAppliedSpec]
	if strings.TrimSpace(lastSpecJSON) == "" {
		return nil, nil
	}

	var spec v1alpha2.VirtualMachineClassSpec
	err := json.Unmarshal([]byte(lastSpecJSON), &spec)
	if err != nil {
		return nil, fmt.Errorf("load spec from JSON: %w", err)
	}
	return &spec, nil
}

// SetLastAppliedClassSpec updates the last-applied-spec annotation with VMClass spec JSON.
func SetLastAppliedClassSpec(kvvm *virtv1.VirtualMachine, class *v1alpha2.VirtualMachineClass) error {
	lastApplied, err := json.Marshal(class.Spec)
	if err != nil {
		return fmt.Errorf("convert spec to JSON: %w", err)
	}

	annotations.AddAnnotation(kvvm, annotations.AnnVMClassLastAppliedSpec, string(lastApplied))
	return nil
}
