/*
Copyright 2026 Flant JSC

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

package vmip

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestValidatorValidateCreateRequiresCIDRs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}

	newValidator := func(virtualMachineCIDRs []string) *Validator {
		cli := fake.NewClientBuilder().WithScheme(scheme).Build()
		ipService, err := service.NewIPAddressService([]string{"10.0.0.0/24"}, cli, nil)
		if err != nil {
			t.Fatalf("NewIPAddressService: %v", err)
		}
		return NewValidator(log.NewNop(), cli, ipService, virtualMachineCIDRs)
	}

	vmip := &v1alpha2.VirtualMachineIPAddress{Spec: v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto}}
	if _, err := newValidator(nil).ValidateCreate(t.Context(), vmip); err == nil {
		t.Fatalf("expected error without CIDRs")
	}
	if _, err := newValidator([]string{"10.0.0.0/24"}).ValidateCreate(t.Context(), vmip); err != nil {
		t.Fatalf("expected success with CIDRs, got: %v", err)
	}
}
