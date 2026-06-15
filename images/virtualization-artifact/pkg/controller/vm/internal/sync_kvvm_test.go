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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	vmservice "github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("SyncKvvmHandler", func() {
	const (
		name      = "vm-sync"
		namespace = "default"
	)

	var (
		ctx          context.Context
		fakeClient   client.WithWatch
		reconcileObj *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState      state.VirtualMachineState
		recorder     *eventrecord.EventRecorderLoggerMock
		featureGates featuregate.FeatureGate
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient = nil
		reconcileObj = nil
		vmState = nil
		featureGates = nil
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...interface{}) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
		}
	})

	AfterEach(func() {
		fakeClient = nil
		reconcileObj = nil
		vmState = nil
		recorder = nil
	})

	makeVM := func(phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)

		vm.Spec.VirtualMachineClassName = "vmclass"
		vm.Spec.CPU.Cores = 2
		vm.Spec.Memory.Size = resource.MustParse("2Gi")
		vm.Spec.RunPolicy = v1alpha2.ManualPolicy
		vm.Spec.VirtualMachineIPAddress = "test-ip"
		vm.Spec.OsType = v1alpha2.GenericOs
		vm.Spec.Disruptions = &v1alpha2.Disruptions{
			RestartApprovalMode: v1alpha2.Manual,
		}

		vm.Status.Phase = phase

		return vm
	}

	// It is like mapPhases in vm/internal/util.go but reversed.
	mapVMPhaseToKVVMPrintableStatus := func(phase v1alpha2.MachinePhase) virtv1.VirtualMachinePrintableStatus {
		switch phase {
		case v1alpha2.MachineRunning:
			return virtv1.VirtualMachineStatusRunning
		case v1alpha2.MachineMigrating:
			return virtv1.VirtualMachineStatusMigrating
		case v1alpha2.MachineStopping:
			return virtv1.VirtualMachineStatusStopping
		case v1alpha2.MachineStopped:
			return virtv1.VirtualMachineStatusStopped
		case v1alpha2.MachineStarting:
			return virtv1.VirtualMachineStatusProvisioning
		case v1alpha2.MachinePending:
			return virtv1.VirtualMachineStatusUnknown
		}
		return ""
	}

	makeKVVM := func(vm *v1alpha2.VirtualMachine) *virtv1.VirtualMachine {
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{},
			},
		}
		kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyAlways)
		kvvm.Spec.Template.Spec.Domain.Devices.Interfaces = []virtv1.Interface{
			{Name: network.NameDefaultInterface},
		}

		// Printable status is required for proper detection if changes are disruptive.
		kvvm.Status.PrintableStatus = mapVMPhaseToKVVMPrintableStatus(vm.Status.Phase)

		Expect(kvbuilder.SetLastAppliedSpec(kvvm, &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				CPU: v1alpha2.CPUSpec{
					Cores: vm.Spec.CPU.Cores,
				},
				Memory: v1alpha2.MemorySpec{
					Size: vm.Spec.Memory.Size,
				},
				VirtualMachineIPAddress: vm.Spec.VirtualMachineIPAddress,
				RunPolicy:               vm.Spec.RunPolicy,
				OsType:                  vm.Spec.OsType,
				VirtualMachineClassName: vm.Spec.VirtualMachineClassName,
				Disruptions: &v1alpha2.Disruptions{
					RestartApprovalMode: vm.Spec.Disruptions.RestartApprovalMode,
				},
			},
		})).To(Succeed())

		Expect(kvbuilder.SetLastAppliedClassSpec(kvvm, &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				CPU: v1alpha2.CPU{
					Type: v1alpha2.CPUTypeHost,
				},
				NodeSelector: v1alpha2.NodeSelector{
					MatchLabels: map[string]string{
						"node1": "node1",
					},
				},
			},
		})).To(Succeed())

		return kvvm
	}

	makeKVVMI := func() *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		return kvvmi
	}

	makeVMIP := func() *v1alpha2.VirtualMachineIPAddress {
		return &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ip",
				Namespace: namespace,
			},
			Spec: v1alpha2.VirtualMachineIPAddressSpec{
				Type:     v1alpha2.VirtualMachineIPAddressTypeStatic,
				StaticIP: "192.168.1.10",
			},
			Status: v1alpha2.VirtualMachineIPAddressStatus{
				Address: "192.168.1.10",
				Phase:   v1alpha2.VirtualMachineIPAddressPhaseAttached,
			},
		}
	}

	makeVMClass := func() *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vmclass",
			}, Spec: v1alpha2.VirtualMachineClassSpec{
				CPU: v1alpha2.CPU{
					Type: v1alpha2.CPUTypeHost,
				},
				NodeSelector: v1alpha2.NodeSelector{
					MatchLabels: map[string]string{
						"node1": "node1",
					},
				},
			},
		}
	}

	reconcile := func() {
		if featureGates == nil {
			featureGates = featuregates.Default()
		}
		h := NewSyncKvvmHandler(nil, fakeClient, recorder, featureGates, vmservice.NewMigrationVolumesService(fakeClient, MakeKVVMFromVMSpec, 10*time.Second))
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = reconcileObj.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	mutateKVVM := func(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
		Expect(kvbuilder.SetLastAppliedSpec(kvvm, &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				CPU: v1alpha2.CPUSpec{
					Cores: 1,
				},
				VirtualMachineIPAddress: vm.Spec.VirtualMachineIPAddress,
				RunPolicy:               vm.Spec.RunPolicy,
				OsType:                  "BIOS",
				VirtualMachineClassName: vm.Spec.VirtualMachineClassName,
				Disruptions: &v1alpha2.Disruptions{
					RestartApprovalMode: vm.Spec.Disruptions.RestartApprovalMode,
				},
			},
		})).To(Succeed())

		Expect(kvbuilder.SetLastAppliedClassSpec(kvvm, &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				CPU: v1alpha2.CPU{
					Type: v1alpha2.CPUTypeHost,
				},
				NodeSelector: v1alpha2.NodeSelector{
					MatchLabels: map[string]string{
						"node2": "node2",
					},
				},
			},
		})).To(Succeed())
	}

	mutateCPUCores := func(cores int) func(fakeClient client.WithWatch, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
		return func(fakeClient client.WithWatch, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
			vm.Spec.CPU.Cores = cores
		}
	}

	mutateMemorySize := func(size string) func(fakeClient client.WithWatch, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
		memSize := resource.MustParse(size)
		return func(fakeClient client.WithWatch, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
			vm.Spec.Memory.Size = memSize
		}
	}

	DescribeTable("AwaitingRestart Condition Tests",
		func(phase v1alpha2.MachinePhase, needChange bool, expectedStatus metav1.ConditionStatus, expectedExistence bool) {
			ip := makeVMIP()
			vmClass := makeVMClass()

			vm := makeVM(phase)
			kvvm := makeKVVM(vm)
			kvvmi := makeKVVMI()

			if needChange {
				mutateKVVM(vm, kvvm)
			}

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass)

			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			awaitCond, awaitExists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
			Expect(awaitExists).To(Equal(expectedExistence))
			if awaitExists {
				Expect(awaitCond.Status).To(Equal(expectedStatus))
			}
		},
		Entry("Running phase with changes", v1alpha2.MachineRunning, true, metav1.ConditionTrue, true),
		Entry("Running phase without changes", v1alpha2.MachineRunning, false, metav1.ConditionUnknown, false),

		Entry("Migrating phase with changes, condition should exist", v1alpha2.MachineMigrating, true, metav1.ConditionTrue, true),
		Entry("Migrating phase without changes, condition should not exist", v1alpha2.MachineMigrating, false, metav1.ConditionUnknown, false),

		Entry("Stopping phase with changes, condition should exist", v1alpha2.MachineStopping, true, metav1.ConditionTrue, true),
		Entry("Stopping phase without changes, condition should not exist", v1alpha2.MachineStopping, false, metav1.ConditionUnknown, false),

		Entry("Stopped phase with changes, shouldn't have condition", v1alpha2.MachineStopped, true, metav1.ConditionUnknown, false),
		Entry("Stopped phase without changes, shouldn't have condition", v1alpha2.MachineStopped, false, metav1.ConditionUnknown, false),

		Entry("Starting phase with changes, shouldn't have condition", v1alpha2.MachineStarting, true, metav1.ConditionUnknown, false),
		Entry("Starting phase without changes, shouldn't have condition", v1alpha2.MachineStarting, false, metav1.ConditionUnknown, false),

		Entry("Pending phase with changes, shouldn't have condition", v1alpha2.MachinePending, true, metav1.ConditionUnknown, false),
		Entry("Pending phase without changes, shouldn't have condition", v1alpha2.MachinePending, false, metav1.ConditionUnknown, false),
	)

	DescribeTable("AwaitingRestart Condition for NonMigratable VM",
		func(phase v1alpha2.MachinePhase, featureGate featuregate.FeatureGate, mutateFn func(fakeClient client.WithWatch, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine), expectedStatus metav1.ConditionStatus, expectedExistence bool) {
			ip := makeVMIP()
			vmClass := makeVMClass()

			vm := makeVM(phase)
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:   vmcondition.TypeMigratable.String(),
				Status: metav1.ConditionFalse,
				Reason: string(vmcondition.ReasonHostDevicesNotMigratable),
			})
			kvvm := makeKVVM(vm)
			kvvmi := makeKVVMI()

			if mutateFn != nil {
				mutateFn(fakeClient, vm, kvvm)
			}

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass)

			featureGates = featureGate

			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			awaitCond, awaitExists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
			Expect(awaitExists).To(Equal(expectedExistence))
			if awaitExists {
				Expect(awaitCond.Status).To(Equal(expectedStatus))
			}
		},
		Entry("should present on cpu.cores change", v1alpha2.MachineRunning, nil, mutateCPUCores(3), metav1.ConditionTrue, true),
		Entry("should present on cpu.cores change when hotplug enabled", v1alpha2.MachineRunning, newFeatureGateEnableCPUHotplug(), mutateCPUCores(3), metav1.ConditionTrue, true),
		Entry("should present on memory.size change", v1alpha2.MachineRunning, nil, mutateMemorySize("4Gi"), metav1.ConditionTrue, true),
		Entry("should present on memory.size change when hotplug enabled", v1alpha2.MachineRunning, newFeatureGateEnableMemoryHotplug(), mutateMemorySize("4Gi"), metav1.ConditionTrue, true),
	)

	DescribeTable("AwaitingRestart Condition for Hotplug VM with Project Quota",
		func(featureGate featuregate.FeatureGate, mutateFn func(fakeClient client.WithWatch, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine), quota *corev1.ResourceQuota, expectedStatus metav1.ConditionStatus, expectedExistence bool, expectedMessages []string) {
			ip := makeVMIP()
			vmClass := makeVMClass()

			vm := makeVM(v1alpha2.MachineRunning)
			kvvm := makeKVVM(vm)
			kvvmi := makeKVVMI()

			if mutateFn != nil {
				mutateFn(fakeClient, vm, kvvm)
			}

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass, quota)
			featureGates = featureGate

			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			awaitCond, awaitExists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
			Expect(awaitExists).To(Equal(expectedExistence))
			if awaitExists {
				Expect(awaitCond.Status).To(Equal(expectedStatus))
				for _, expectedMessage := range expectedMessages {
					Expect(awaitCond.Message).To(ContainSubstring(expectedMessage))
				}
			}
		},
		Entry(
			"should present on cpu hotplug when quota is insufficient during migration",
			newFeatureGateEnableCPUHotplug(),
			mutateCPUCores(4),
			newResourceQuota(resource.MustParse("6"), resource.MustParse("32Gi"), resource.MustParse("3"), resource.MustParse("2Gi")),
			metav1.ConditionTrue,
			true,
			[]string{`project quota "project-quota" has insufficient requests.cpu`},
		),
		Entry(
			"should not present on cpu hotplug when quota is sufficient during migration",
			newFeatureGateEnableCPUHotplug(),
			mutateCPUCores(4),
			newResourceQuota(resource.MustParse("8"), resource.MustParse("32Gi"), resource.MustParse("3"), resource.MustParse("2Gi")),
			metav1.ConditionUnknown,
			false,
			nil,
		),
		Entry(
			"should present on memory hotplug when quota is insufficient during migration",
			newFeatureGateEnableMemoryHotplug(),
			mutateMemorySize("4Gi"),
			newResourceQuota(resource.MustParse("8"), resource.MustParse("5Gi"), resource.MustParse("2"), resource.MustParse("2Gi")),
			metav1.ConditionTrue,
			true,
			[]string{`project quota "project-quota" has insufficient requests.memory`},
		),
		Entry(
			"should present joined message when cpu and memory quota are insufficient during migration",
			newFeatureGateEnableResourceHotplug(),
			func(_ client.WithWatch, vm *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) {
				vm.Spec.CPU.Cores = 4
				vm.Spec.Memory.Size = resource.MustParse("4Gi")
			},
			newResourceQuota(resource.MustParse("6"), resource.MustParse("5Gi"), resource.MustParse("3"), resource.MustParse("2Gi")),
			metav1.ConditionTrue,
			true,
			[]string{
				`project quota "project-quota" has insufficient requests.cpu`,
				`project quota "project-quota" has insufficient requests.memory`,
			},
		),
	)

	DescribeTable("ConfigurationApplied Condition Tests",
		func(phase v1alpha2.MachinePhase, notReady bool, expectedStatus metav1.ConditionStatus, expectedExistence bool) {
			ip := makeVMIP()
			vmClass := makeVMClass()

			vm := makeVM(phase)
			if notReady {
				vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
					Type:   vmcondition.TypeBlockDevicesReady.String(),
					Status: metav1.ConditionFalse,
					Reason: "BlockDevicesNotReady",
				})
			}
			kvvm := makeKVVM(vm)
			kvvmi := makeKVVMI()

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			confAppliedCond, confAppliedExists := conditions.GetCondition(vmcondition.TypeConfigurationApplied, newVM.Status.Conditions)
			Expect(confAppliedExists).To(Equal(expectedExistence))
			if confAppliedExists {
				Expect(confAppliedCond.Status).To(Equal(expectedStatus))
			}
		},
		Entry("Running phase with changes applied", v1alpha2.MachineRunning, false, metav1.ConditionUnknown, false),
		Entry("Running phase with changes not applied", v1alpha2.MachineRunning, true, metav1.ConditionFalse, true),

		Entry("Migrating phase with changes applied, condition should not exist", v1alpha2.MachineMigrating, false, metav1.ConditionUnknown, false),
		Entry("Migrating phase with changes not applied, condition should exist", v1alpha2.MachineMigrating, true, metav1.ConditionFalse, true),

		Entry("Stopping phase with changes applied, condition should not exist", v1alpha2.MachineStopping, false, metav1.ConditionUnknown, false),
		Entry("Stopping phase with changes not applied, condition should exist", v1alpha2.MachineStopping, true, metav1.ConditionFalse, true),

		Entry("Stopped phase with changes applied, condition should not exist", v1alpha2.MachineStopped, false, metav1.ConditionUnknown, false),
		Entry("Stopped phase with changes not applied, condition should not exist", v1alpha2.MachineStopped, true, metav1.ConditionUnknown, false),

		Entry("Starting phase with changes applied, condition should not exist", v1alpha2.MachineStarting, false, metav1.ConditionUnknown, false),
		Entry("Starting phase with changes not applied, condition should not exist", v1alpha2.MachineStarting, true, metav1.ConditionUnknown, false),

		Entry("Pending phase with changes applied, condition should not exist", v1alpha2.MachinePending, false, metav1.ConditionUnknown, false),
		Entry("Pending phase with changes not applied, condition should not exist", v1alpha2.MachinePending, true, metav1.ConditionUnknown, false),
	)

	It("keeps ConfigurationApplied False and requeues while SDN is not ready", func() {
		ip := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "test-ip", Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeStatic, StaticIP: "192.168.1.10"},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: "192.168.1.10", Phase: v1alpha2.VirtualMachineIPAddressPhaseAttached},
		}
		vmClass := &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Name: "vmclass"},
			Spec: v1alpha2.VirtualMachineClassSpec{
				CPU:          v1alpha2.CPU{Type: v1alpha2.CPUTypeHost},
				NodeSelector: v1alpha2.NodeSelector{MatchLabels: map[string]string{"node1": "node1"}},
			},
		}

		vm := makeVM(v1alpha2.MachineRunning)
		kvvm := makeKVVM(vm)
		// Drop the interface so the desired network is out of sync with the KVVM,
		// and provide no pod so SDN reports the interface as not ready.
		kvvm.Spec.Template.Spec.Domain.Devices.Interfaces = nil

		fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, ip, vmClass)

		h := NewSyncKvvmHandler(nil, fakeClient, recorder, featuregates.Default(), vmservice.NewMigrationVolumesService(fakeClient, MakeKVVMFromVMSpec, 10*time.Second))
		result, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(reconcileObj.Update(context.Background())).To(Succeed())

		updatedVM := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), updatedVM)).To(Succeed())
		cond, exists := conditions.GetCondition(vmcondition.TypeConfigurationApplied, updatedVM.Status.Conditions)
		Expect(exists).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonConfigurationNotApplied.String()))
	})

	DescribeTable("isPlacementPolicyChanged",
		func(path string, expected bool) {
			h := &SyncKvvmHandler{}
			changes := vmchange.SpecChanges{}
			changes.Add(vmchange.FieldChange{Path: path, CurrentValue: "old", DesiredValue: "new"})

			Expect(h.isPlacementPolicyChanged(changes)).To(Equal(expected))
		},
		Entry("vm tolerations change", "tolerations", true),
		Entry("vmclass tolerations change", "VirtualMachineClass:spec.tolerations", true),
		Entry("vmclass nodeSelector change", "VirtualMachineClass:spec.nodeSelector", true),
		Entry("vmclass name change", "virtualMachineClassName", true),
		Entry("cpu change is not a placement policy", "cpu.cores", false),
	)
})

func newFeatureGate(enabled ...featuregate.Feature) featuregate.FeatureGate {
	GinkgoHelper()

	gate, setFromMap, err := featuregates.NewUnlocked()
	Expect(err).NotTo(HaveOccurred())
	featureMap := map[string]bool{}
	for _, feature := range enabled {
		featureMap[string(feature)] = true
	}
	err = setFromMap(featureMap)
	Expect(err).NotTo(HaveOccurred())

	return gate
}

func newFeatureGateEnableCPUHotplug() featuregate.FeatureGate {
	return newFeatureGate(featuregates.HotplugCPUWithLiveMigration)
}

func newFeatureGateEnableMemoryHotplug() featuregate.FeatureGate {
	return newFeatureGate(featuregates.HotplugMemoryWithLiveMigration)
}

func newFeatureGateEnableResourceHotplug() featuregate.FeatureGate {
	return newFeatureGate(featuregates.HotplugCPUWithLiveMigration, featuregates.HotplugMemoryWithLiveMigration)
}

func newResourceQuota(cpuHard, memoryHard, cpuUsed, memoryUsed resource.Quantity) *corev1.ResourceQuota {
	const quotaNamespace = "default"

	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "project-quota",
			Namespace: quotaNamespace,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    cpuHard,
				corev1.ResourceRequestsMemory: memoryHard,
			},
		},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    cpuHard,
				corev1.ResourceRequestsMemory: memoryHard,
			},
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    cpuUsed,
				corev1.ResourceRequestsMemory: memoryUsed,
			},
		},
	}
}
