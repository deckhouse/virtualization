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
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("LifeCycleHandler paused virtual machine", func() {
	DescribeTable("should report the paused virtual machine as not running",
		func(pausedReason, expectedMessage string) {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePause,
				},
			}
			kvvmi := &virtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vm.Name,
					Namespace: vm.Namespace,
				},
				Status: virtv1.VirtualMachineInstanceStatus{
					Conditions: []virtv1.VirtualMachineInstanceCondition{
						{
							Type:   virtv1.VirtualMachineInstanceReady,
							Status: corev1.ConditionFalse,
							Reason: "ReadinessGatesNotReady",
						},
						{
							Type:   virtv1.VirtualMachineInstancePaused,
							Status: corev1.ConditionTrue,
							Reason: pausedReason,
						},
					},
				},
			}

			fakeClient, err := testutil.NewFakeClientWithObjects(vm, kvvmi)
			Expect(err).NotTo(HaveOccurred())
			handler := NewLifeCycleHandler(fakeClient, nil)

			Expect(handler.syncRunning(context.Background(), vm, nil, kvvmi, nil, slog.Default())).To(Succeed())

			running, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
			Expect(running.Status).To(Equal(metav1.ConditionFalse))
			Expect(running.Reason).To(Equal(vmcondition.ReasonVirtualMachinePaused.String()))
			Expect(running.Message).To(Equal(expectedMessage))
		},
		Entry("when paused on request", pausedByUserReason,
			"The virtual machine is paused on request."),
		Entry("when paused to complete the live migration", pausedByMigrationMonitorReason,
			"The virtual machine is paused to complete the live migration: its memory changes faster than it can be transferred to the destination node. It resumes once the migration ends."),
		Entry("when paused due to a disk I/O error", pausedIOErrorReason,
			"The virtual machine is paused due to a disk I/O error. Check that the storage of its disks is available and has free space."),
		Entry("when paused for another reason", "SomethingElse",
			"The virtual machine is paused."),
	)
})
