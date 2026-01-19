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

package vm

import (
	"testing"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCalculateCoresAndSockets(t *testing.T) {
	tests := []struct {
		desiredCores int
		sockets      int
		cores        int
	}{
		{-1, 1, 1},
		{1, 1, 1},
		{2, 1, 2},
		{3, 1, 3},
		{4, 1, 4},
		{5, 1, 5},
		{15, 1, 15},
		{16, 1, 16},

		{18, 2, 9},
		{19, 2, 10},
		{20, 2, 10},
		{31, 2, 16},
		{32, 2, 16},

		{36, 4, 9},
		{37, 4, 10},
		{40, 4, 10},
		{60, 4, 15},
		{63, 4, 16},
		{64, 4, 16},

		{72, 8, 9},
		{76, 8, 10},
		{80, 8, 10},
		{248, 8, 31},
		{256, 8, 32},
		{252, 8, 32},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			sockets, cores := CalculateCoresAndSockets(test.desiredCores)
			if cores != test.cores && sockets != test.sockets {
				t.Errorf("For %d cores, expected %d sockets and %d cores, got  %d sockets and %d cores", test.desiredCores, test.sockets, test.cores, sockets, cores)
			}
		})
	}
}

func TestGetActivePodName(t *testing.T) {
	// Exists
	// Given
	vm := &v1alpha2.VirtualMachine{
		Status: v1alpha2.VirtualMachineStatus{
			VirtualMachinePods: []v1alpha2.VirtualMachinePod{
				{
					Name:   "test-not-active",
					Active: false,
				},
				{
					Name:   "test-active",
					Active: true,
				},
			},
		},
	}

	// When
	podName, ok := GetActivePodName(vm)

	// Then
	if !ok {
		t.Errorf("must return pod name")
	}
	if podName != "test-active" {
		t.Errorf("must return test-active pod name, not %s", podName)
	}

	// Not exists active pod
	// Given
	vm = &v1alpha2.VirtualMachine{
		Status: v1alpha2.VirtualMachineStatus{
			VirtualMachinePods: []v1alpha2.VirtualMachinePod{
				{
					Name:   "test-not-active",
					Active: false,
				},
				{
					Name:   "test-not-active-2",
					Active: false,
				},
			},
		},
	}

	// When
	podName, ok = GetActivePodName(vm)

	// Then
	if ok {
		t.Errorf("must not return pod name")
	}
	if podName != "" {
		t.Errorf("must return empty pod name, not %s", podName)
	}
}
