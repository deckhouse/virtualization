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
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("LifeCycleHandler", func() {
	Describe("syncLastStartTime", func() {
		It("sets lastStartTime from the Running condition last transition time", func() {
			transitionTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
			vm := newVMWithRunningCondition(transitionTime)

			syncLastStartTime(vm, nil)

			Expect(vm.Status.Stats).NotTo(BeNil())
			Expect(vm.Status.Stats.LastStartTime).NotTo(BeNil())
			Expect(vm.Status.Stats.LastStartTime.Time).To(Equal(transitionTime.Time))
		})

		It("sets lastStartTime from the VMI Running phase transition if it differs from the Running condition transition by more than ten minutes", func() {
			conditionTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 20, 0, 0, time.UTC))
			vmiRunningTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
			vm := newVMWithRunningCondition(conditionTime)
			kvvmi := newKVVMIWithRunningPhaseTransition(vmiRunningTime)

			syncLastStartTime(vm, kvvmi)

			Expect(vm.Status.Stats).NotTo(BeNil())
			Expect(vm.Status.Stats.LastStartTime).NotTo(BeNil())
			Expect(vm.Status.Stats.LastStartTime.Time).To(Equal(vmiRunningTime.Time))
		})

		It("sets lastStartTime from the VMI Running phase transition if it is newer than the Running condition transition by more than ten minutes", func() {
			conditionTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
			vmiRunningTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 20, 0, 0, time.UTC))
			vm := newVMWithRunningCondition(conditionTime)
			kvvmi := newKVVMIWithRunningPhaseTransition(vmiRunningTime)

			syncLastStartTime(vm, kvvmi)

			Expect(vm.Status.Stats).NotTo(BeNil())
			Expect(vm.Status.Stats.LastStartTime).NotTo(BeNil())
			Expect(vm.Status.Stats.LastStartTime.Time).To(Equal(vmiRunningTime.Time))
		})

		It("sets lastStartTime from the Running condition when the VMI Running phase transition does not differ by more than ten minutes", func() {
			conditionTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 9, 0, 0, time.UTC))
			vmiRunningTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
			vm := newVMWithRunningCondition(conditionTime)
			kvvmi := newKVVMIWithRunningPhaseTransition(vmiRunningTime)

			syncLastStartTime(vm, kvvmi)

			Expect(vm.Status.Stats).NotTo(BeNil())
			Expect(vm.Status.Stats.LastStartTime).NotTo(BeNil())
			Expect(vm.Status.Stats.LastStartTime.Time).To(Equal(conditionTime.Time))
		})

		DescribeTable("clears lastStartTime when the VM is not running",
			func(conditions []metav1.Condition) {
				lastStartTime := metav1.NewTime(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
				vm := &v1alpha2.VirtualMachine{
					Status: v1alpha2.VirtualMachineStatus{
						Stats:      &v1alpha2.VirtualMachineStats{LastStartTime: &lastStartTime},
						Conditions: conditions,
					},
				}

				syncLastStartTime(vm, nil)

				Expect(vm.Status.Stats).NotTo(BeNil())
				Expect(vm.Status.Stats.LastStartTime).To(BeNil())
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

func newVMWithRunningCondition(transitionTime metav1.Time) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
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
}

func newKVVMIWithRunningPhaseTransition(transitionTime metav1.Time) *virtv1.VirtualMachineInstance {
	return &virtv1.VirtualMachineInstance{
		Status: virtv1.VirtualMachineInstanceStatus{
			PhaseTransitionTimestamps: []virtv1.VirtualMachineInstancePhaseTransitionTimestamp{
				{
					Phase:                    virtv1.Running,
					PhaseTransitionTimestamp: transitionTime,
				},
			},
		},
	}
}
