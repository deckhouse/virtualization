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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCalculateCoresAndSockets(t *testing.T) {
	tests := []struct {
		desiredCores   int
		sockets        int
		cores          int
		coresPerSocket int
	}{
		{-1, 1, 1, 16},
		{1, 1, 1, 16},
		{2, 1, 2, 16},
		{3, 1, 3, 16},
		{4, 1, 4, 16},
		{5, 1, 5, 16},
		{15, 1, 15, 16},
		{16, 1, 16, 16},

		{18, 2, 9, 16},
		{19, 2, 10, 16},
		{20, 2, 10, 16},
		{31, 2, 16, 16},
		{32, 2, 16, 16},

		{36, 4, 9, 16},
		{37, 4, 10, 16},
		{40, 4, 10, 16},
		{60, 4, 15, 16},
		{63, 4, 16, 16},
		{64, 4, 16, 16},

		{72, 8, 9, 31},
		{76, 8, 10, 31},
		{80, 8, 10, 31},
		{248, 8, 31, 31},
		{252, 8, 32, 31},
		{256, 8, 32, 31},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			sockets, cores, coresPerSocket := CalculateCoresAndSockets(test.desiredCores)
			if cores != test.cores && sockets != test.sockets {
				t.Errorf("For %d cores, expected topology %ds/%dc/%dmax, got %ds/%dc/%dmax",
					test.desiredCores,
					test.sockets, test.cores, test.coresPerSocket,
					sockets, cores, coresPerSocket,
				)
			}
		})
	}
}

var _ = Describe("GetActivePodName", func() {
	It("should return the name of the active pod if it exists", func() {
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

		podName, ok := GetActivePodName(vm)
		Expect(ok).To(BeTrue(), "must return pod name")
		Expect(podName).To(Equal("test-active"), "must return test-active pod name")
	})

	It("should not return pod name if no pod is active", func() {
		vm := &v1alpha2.VirtualMachine{
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

		podName, ok := GetActivePodName(vm)
		Expect(ok).To(BeFalse(), "must not return pod name")
		Expect(podName).To(Equal(""), "must return empty pod name")
	})
})
