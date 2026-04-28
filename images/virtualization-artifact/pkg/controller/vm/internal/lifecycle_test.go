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

package internal

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("LifeCycleHandler", func() {
	Describe("syncStartedAt", func() {
		It("sets startedAt from the Running condition last transition time", func() {
			transitionTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
			vm := &v1alpha2.VirtualMachine{
				Status: v1alpha2.VirtualMachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:               vmcondition.TypeRunning.String(),
							Status:             metav1.ConditionTrue,
							LastTransitionTime: transitionTime,
						},
					},
				},
			}

			syncStartedAt(vm)

			Expect(vm.Status.Stats).NotTo(BeNil())
			Expect(vm.Status.Stats.StartedAt).NotTo(BeNil())
			Expect(vm.Status.Stats.StartedAt.Time).To(Equal(transitionTime.Time))
		})

		It("keeps existing startedAt while the VM remains running", func() {
			startedAt := metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
			transitionTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 10, 0, 0, time.UTC))
			vm := &v1alpha2.VirtualMachine{
				Status: v1alpha2.VirtualMachineStatus{
					Stats: &v1alpha2.VirtualMachineStats{StartedAt: &startedAt},
					Conditions: []metav1.Condition{
						{
							Type:               vmcondition.TypeRunning.String(),
							Status:             metav1.ConditionTrue,
							LastTransitionTime: transitionTime,
						},
					},
				},
			}

			syncStartedAt(vm)

			Expect(vm.Status.Stats.StartedAt).NotTo(BeNil())
			Expect(vm.Status.Stats.StartedAt.Time).To(Equal(startedAt.Time))
		})

		DescribeTable("clears startedAt when the VM is not running",
			func(conditions []metav1.Condition) {
				startedAt := metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
				vm := &v1alpha2.VirtualMachine{
					Status: v1alpha2.VirtualMachineStatus{
						Stats:      &v1alpha2.VirtualMachineStats{StartedAt: &startedAt},
						Conditions: conditions,
					},
				}

				syncStartedAt(vm)

				Expect(vm.Status.Stats).NotTo(BeNil())
				Expect(vm.Status.Stats.StartedAt).To(BeNil())
			},
			Entry("without the Running condition", nil),
			Entry("with the Running condition set to False", []metav1.Condition{
				{
					Type:               vmcondition.TypeRunning.String(),
					Status:             metav1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)),
				},
			}),
		)
	})
})
