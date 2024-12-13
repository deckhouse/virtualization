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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("SyncKvvmHandler", func() {
	const (
		name      = "vm-sync"
		namespace = "default"
	)

	var (
		ctx        context.Context
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
		recorder   *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient = nil
		resource = nil
		vmState = nil
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...interface{}) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
		}
	})

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
		recorder = nil
	})

	newVM := func(phase virtv2.MachinePhase) *virtv2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Status.Phase = phase
		vm.Spec.VirtualMachineClassName = "vmclass"
		vm.Spec.CPU.Cores = 2
		vm.Spec.RunPolicy = virtv2.ManualPolicy
		vm.Spec.VirtualMachineIPAddress = "test-ip"
		vm.Spec.OsType = virtv2.GenericOs
		vm.Spec.Disruptions = &virtv2.Disruptions{
			RestartApprovalMode: virtv2.Manual,
		}

		return vm
	}

	newKVVM := func(vm *virtv2.VirtualMachine) *virtv1.VirtualMachine {
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{},
			},
		}
		kvvm.Spec.RunStrategy = pointer.GetPointer(virtv1.RunStrategyAlways)

		Expect(kvbuilder.SetLastAppliedSpec(kvvm, &virtv2.VirtualMachine{
			Spec: virtv2.VirtualMachineSpec{
				CPU: virtv2.CPUSpec{
					Cores: vm.Spec.CPU.Cores,
				},
				VirtualMachineIPAddress: vm.Spec.VirtualMachineIPAddress,
				RunPolicy:               vm.Spec.RunPolicy,
				OsType:                  vm.Spec.OsType,
				VirtualMachineClassName: vm.Spec.VirtualMachineClassName,
				Disruptions: &virtv2.Disruptions{
					RestartApprovalMode: vm.Spec.Disruptions.RestartApprovalMode,
				},
			},
		})).To(Succeed())

		Expect(kvbuilder.SetLastAppliedClassSpec(kvvm, &virtv2.VirtualMachineClass{
			Spec: virtv2.VirtualMachineClassSpec{
				CPU: virtv2.CPU{
					Type: virtv2.CPUTypeHost,
				},
				NodeSelector: virtv2.NodeSelector{
					MatchLabels: map[string]string{
						"node1": "node1",
					},
				},
			},
		})).To(Succeed())

		return kvvm
	}

	newKVVMI := func() *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		return kvvmi
	}

	reconcile := func() {
		h := NewSyncKvvmHandler(nil, fakeClient, recorder)
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	mutateKVVM := func(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
		Expect(kvbuilder.SetLastAppliedSpec(kvvm, &virtv2.VirtualMachine{
			Spec: virtv2.VirtualMachineSpec{
				CPU: virtv2.CPUSpec{
					Cores: 1,
				},
				VirtualMachineIPAddress: vm.Spec.VirtualMachineIPAddress,
				RunPolicy:               vm.Spec.RunPolicy,
				OsType:                  "BIOS",
				VirtualMachineClassName: vm.Spec.VirtualMachineClassName,
				Disruptions: &virtv2.Disruptions{
					RestartApprovalMode: vm.Spec.Disruptions.RestartApprovalMode,
				},
			},
		})).To(Succeed())

		Expect(kvbuilder.SetLastAppliedClassSpec(kvvm, &virtv2.VirtualMachineClass{
			Spec: virtv2.VirtualMachineClassSpec{
				CPU: virtv2.CPU{
					Type: virtv2.CPUTypeHost,
				},
				NodeSelector: virtv2.NodeSelector{
					MatchLabels: map[string]string{
						"node2": "node2",
					},
				},
			},
		})).To(Succeed())
	}

	DescribeTable("AwaitingRestart Condition Tests",
		func(phase virtv2.MachinePhase, needChange bool, expectedStatus metav1.ConditionStatus, expectedExistence bool) {
			ip := &virtv2.VirtualMachineIPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ip",
					Namespace: namespace,
				},
				Spec: virtv2.VirtualMachineIPAddressSpec{
					Type:     virtv2.VirtualMachineIPAddressTypeStatic,
					StaticIP: "192.168.1.10",
				},
				Status: virtv2.VirtualMachineIPAddressStatus{
					Address: "192.168.1.10",
					Phase:   virtv2.VirtualMachineIPAddressPhaseAttached,
				},
			}

			vmClass := &virtv2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vmclass",
				}, Spec: virtv2.VirtualMachineClassSpec{
					CPU: virtv2.CPU{
						Type: virtv2.CPUTypeHost,
					},
					NodeSelector: virtv2.NodeSelector{
						MatchLabels: map[string]string{
							"node1": "node1",
						},
					},
				},
			}

			vm := newVM(phase)
			kvvm := newKVVM(vm)
			kvvmi := newKVVMI()

			if needChange {
				mutateKVVM(vm, kvvm)
			}

			fakeClient, resource, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass)

			reconcile()

			newVM := &virtv2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			awaitCond, awaitExists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
			Expect(awaitExists).To(Equal(expectedExistence))
			if awaitExists {
				Expect(awaitCond.Status).To(Equal(expectedStatus))
			}
		},
		Entry("Running phase with changes", virtv2.MachineRunning, true, metav1.ConditionTrue, true),
		Entry("Running phase without changes", virtv2.MachineRunning, false, metav1.ConditionUnknown, false),

		Entry("Migrating phase with changes, condition should exist", virtv2.MachineMigrating, true, metav1.ConditionTrue, true),
		Entry("Migrating phase without changes, condition should not exist", virtv2.MachineMigrating, false, metav1.ConditionUnknown, false),

		Entry("Stopping phase with changes, condition should exist", virtv2.MachineStopping, true, metav1.ConditionTrue, true),
		Entry("Stopping phase without changes, condition should not exist", virtv2.MachineStopping, false, metav1.ConditionUnknown, false),

		Entry("Stopped phase with changes, shouldn't have condition", virtv2.MachineStopped, true, metav1.ConditionUnknown, false),
		Entry("Stopped phase without changes, shouldn't have condition", virtv2.MachineStopped, false, metav1.ConditionUnknown, false),

		Entry("Starting phase with changes, shouldn't have condition", virtv2.MachineStarting, true, metav1.ConditionUnknown, false),
		Entry("Starting phase without changes, shouldn't have condition", virtv2.MachineStarting, false, metav1.ConditionUnknown, false),

		Entry("Pending phase with changes, shouldn't have condition", virtv2.MachinePending, true, metav1.ConditionUnknown, false),
		Entry("Pending phase without changes, shouldn't have condition", virtv2.MachinePending, false, metav1.ConditionUnknown, false),
	)

	DescribeTable("ConfigurationApplied Condition Tests",
		func(phase virtv2.MachinePhase, notReady bool, expectedStatus metav1.ConditionStatus, expectedExistence bool) {
			ip := &virtv2.VirtualMachineIPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ip",
					Namespace: namespace,
				},
				Spec: virtv2.VirtualMachineIPAddressSpec{
					Type:     virtv2.VirtualMachineIPAddressTypeStatic,
					StaticIP: "192.168.1.10",
				},
				Status: virtv2.VirtualMachineIPAddressStatus{
					Address: "192.168.1.10",
					Phase:   virtv2.VirtualMachineIPAddressPhaseAttached,
				},
			}

			vmClass := &virtv2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vmclass",
				}, Spec: virtv2.VirtualMachineClassSpec{
					CPU: virtv2.CPU{
						Type: virtv2.CPUTypeHost,
					},
					NodeSelector: virtv2.NodeSelector{
						MatchLabels: map[string]string{
							"node1": "node1",
						},
					},
				},
			}

			vm := newVM(phase)
			if notReady {
				vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
					Type:   vmcondition.TypeBlockDevicesReady.String(),
					Status: metav1.ConditionFalse,
					Reason: "BlockDevicesNotReady",
				})
			}
			kvvm := newKVVM(vm)

			fakeClient, resource, vmState = setupEnvironment(vm, kvvm, ip, vmClass)
			reconcile()

			newVM := &virtv2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			confAppliedCond, confAppliedExists := conditions.GetCondition(vmcondition.TypeConfigurationApplied, newVM.Status.Conditions)
			Expect(confAppliedExists).To(Equal(expectedExistence))
			if confAppliedExists {
				Expect(confAppliedCond.Status).To(Equal(expectedStatus))
			}
		},
		Entry("Running phase with changes applied", virtv2.MachineRunning, false, metav1.ConditionUnknown, false),
		Entry("Running phase with changes not applied", virtv2.MachineRunning, true, metav1.ConditionFalse, true),

		Entry("Migrating phase with changes applied, condition should not exist", virtv2.MachineMigrating, false, metav1.ConditionUnknown, false),
		Entry("Migrating phase with changes not applied, condition should exist", virtv2.MachineMigrating, true, metav1.ConditionFalse, true),

		Entry("Stopping phase with changes applied, condition should not exist", virtv2.MachineStopping, false, metav1.ConditionUnknown, false),
		Entry("Stopping phase with changes not applied, condition should exist", virtv2.MachineStopping, true, metav1.ConditionFalse, true),

		Entry("Stopped phase with changes applied, condition should not exist", virtv2.MachineStopped, false, metav1.ConditionUnknown, false),
		Entry("Stopped phase with changes not applied, condition should not exist", virtv2.MachineStopped, true, metav1.ConditionUnknown, false),

		Entry("Starting phase with changes applied, condition should not exist", virtv2.MachineStarting, false, metav1.ConditionUnknown, false),
		Entry("Starting phase with changes not applied, condition should not exist", virtv2.MachineStarting, true, metav1.ConditionUnknown, false),

		Entry("Pending phase with changes applied, condition should not exist", virtv2.MachinePending, false, metav1.ConditionUnknown, false),
		Entry("Pending phase with changes not applied, condition should not exist", virtv2.MachinePending, true, metav1.ConditionUnknown, false),
	)
})
