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

package validators

import (
	"fmt"
	"testing"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCpuCountValidate(t *testing.T) {
	tests := []struct {
		desiredCores int
		valid        bool
	}{
		{1, true},
		{2, true},
		{3, true},
		{4, true},
		{5, true},
		{15, true},
		{16, true},

		{18, true},
		{19, false},
		{20, true},
		{31, false},
		{32, true},

		{36, true},
		{37, false},
		{40, true},
		{60, true},
		{63, false},
		{64, true},

		{72, true},
		{76, false},
		{80, true},
		{248, true},
		{256, true},
		{252, false},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("TestCase%d", i), func(t *testing.T) {
			vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{CPU: v1alpha2.CPUSpec{Cores: test.desiredCores}}}
			cpuCountValidator := NewCPUCountValidator()

			_, err := cpuCountValidator.Validate(vm)

			if test.valid && err != nil {
				t.Errorf("For %d cores, expected valid, but validation failed", test.desiredCores)
			}

			if !test.valid && err == nil {
				t.Errorf("For %d cores, expected not valid, but validation succeeded", test.desiredCores)
			}
		})
	}
}
