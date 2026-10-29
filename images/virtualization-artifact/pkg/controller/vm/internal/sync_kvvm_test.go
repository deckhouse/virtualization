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
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/hotplugmigration"
	vmservice "github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmchange"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
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

	makeResizingKVVMI := func(reason, message string, conditionType virtv1.VirtualMachineInstanceConditionType) *virtv1.VirtualMachineInstance {
		kvvmi := makeKVVMI()
		kvvmi.Annotations = map[string]string{
			virtv1.VirtualMachineInstanceInPlaceResizeInProgressAnn: "true",
		}
		kvvmi.Status.Conditions = []virtv1.VirtualMachineInstanceCondition{
			{
				Type:    virtv1.VirtualMachineInstancePodResourceResizeInProgress,
				Status:  corev1.ConditionTrue,
				Reason:  reason,
				Message: message,
			},
		}
		if conditionType != "" {
			kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   conditionType,
				Status: corev1.ConditionTrue,
			})
		}

		return kvvmi
	}

	// makeUnconfirmedResizeKVVMI returns a KVVMI whose in-place resize has been requested but never
	// confirmed by the runtime: the marker is set, the resize condition is absent.
	makeUnconfirmedResizeKVVMI := func(conditionType virtv1.VirtualMachineInstanceConditionType) *virtv1.VirtualMachineInstance {
		kvvmi := makeResizingKVVMI("", "", conditionType)

		conds := make([]virtv1.VirtualMachineInstanceCondition, 0, len(kvvmi.Status.Conditions))
		for _, cond := range kvvmi.Status.Conditions {
			if cond.Type != virtv1.VirtualMachineInstancePodResourceResizeInProgress {
				conds = append(conds, cond)
			}
		}
		kvvmi.Status.Conditions = conds

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

	// mutateCoreFraction moves the core fraction from applied to desired; for "Auto" the autoscaler
	// asks for recommended.
	mutateCoreFraction := func(applied, desired, recommended string) func(fakeClient client.WithWatch, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
		return func(fakeClient client.WithWatch, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) {
			lastApplied, err := kvbuilder.LoadLastAppliedSpec(kvvm)
			Expect(err).NotTo(HaveOccurred())
			lastApplied.CPU.CoreFraction = applied
			Expect(kvbuilder.SetLastAppliedSpec(kvvm, &v1alpha2.VirtualMachine{Spec: *lastApplied})).To(Succeed())
			vm.Spec.CPU.CoreFraction = desired
			sizingpolicy.SetRecommendedCoreFraction(vm, recommended)
		}
	}

	Context("restore-power-state intent on KVVM creation", func() {
		It("requests start for a VM that was running before restore and clears the intent", func() {
			vm := makeVM(v1alpha2.MachineStopped)
			vm.Spec.RunPolicy = v1alpha2.ManualPolicy
			vm.Annotations = map[string]string{annotations.AnnVMRestorePowerState: string(v1alpha2.MachineRunning)}

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, makeVMIP(), makeVMClass())

			reconcile()

			kvvm := &virtv1.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), kvvm)).To(Succeed())
			Expect(kvvm.Annotations).To(HaveKeyWithValue(annotations.AnnVMStartRequested, "true"))

			newVM := &v1alpha2.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
			Expect(newVM.Annotations).NotTo(HaveKey(annotations.AnnVMRestorePowerState))
		})

		It("keeps AlwaysOnUnlessStoppedManually VM stopped and clears the intent", func() {
			vm := makeVM(v1alpha2.MachineStopped)
			vm.Spec.RunPolicy = v1alpha2.AlwaysOnUnlessStoppedManually
			vm.Annotations = map[string]string{annotations.AnnVMRestorePowerState: string(v1alpha2.MachineStopped)}

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, makeVMIP(), makeVMClass())

			reconcile()

			kvvm := &virtv1.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), kvvm)).To(Succeed())
			Expect(kvvm.Spec.RunStrategy).NotTo(BeNil())
			Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))

			newVM := &v1alpha2.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
			Expect(newVM.Annotations).NotTo(HaveKey(annotations.AnnVMRestorePowerState))
		})

		It("keeps the restore intent when KVVM creation fails so a retry can honor it", func() {
			vm := makeVM(v1alpha2.MachineStopped)
			vm.Spec.RunPolicy = v1alpha2.AlwaysOnUnlessStoppedManually
			vm.Annotations = map[string]string{annotations.AnnVMRestorePowerState: string(v1alpha2.MachineStopped)}

			createFails := interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*virtv1.VirtualMachine); ok {
						return errors.New("create rejected")
					}
					return c.Create(ctx, obj, opts...)
				},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(createFails, vm, makeVMIP(), makeVMClass())
			Expect(err).NotTo(HaveOccurred())

			reconcileObj = reconciler.NewResource(client.ObjectKeyFromObject(vm), fakeClient,
				func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
				func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status })
			Expect(reconcileObj.Fetch(ctx)).To(Succeed())
			vmState = state.New(fakeClient, reconcileObj)

			h := NewSyncKvvmHandler(nil, fakeClient, recorder, featuregates.Default(), vmservice.NewMigrationVolumesService(fakeClient, MakeKVVMFromVMSpec, 10*time.Second))
			_, handleErr := h.Handle(ctx, vmState)
			Expect(handleErr).To(HaveOccurred())
			Expect(reconcileObj.Update(ctx)).To(Succeed())

			newVM := &v1alpha2.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
			Expect(newVM.Annotations).To(HaveKeyWithValue(annotations.AnnVMRestorePowerState, string(v1alpha2.MachineStopped)))
		})

		It("starts AlwaysOnUnlessStoppedManually VM on create without the keep-stopped intent", func() {
			vm := makeVM(v1alpha2.MachineStopped)
			vm.Spec.RunPolicy = v1alpha2.AlwaysOnUnlessStoppedManually

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, makeVMIP(), makeVMClass())

			reconcile()

			kvvm := &virtv1.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), kvvm)).To(Succeed())
			Expect(kvvm.Spec.RunStrategy).NotTo(BeNil())
			Expect(*kvvm.Spec.RunStrategy).To(Equal(virtv1.RunStrategyAlways))
		})
	})

	DescribeTable("switching the run policy of a stopped VM",
		func(prevRunPolicy v1alpha2.RunPolicy, prevRunStrategy virtv1.VirtualMachineRunStrategy, runPolicy v1alpha2.RunPolicy, expectStart bool) {
			vm := makeVM(v1alpha2.MachineStopped)
			vm.Spec.RunPolicy = prevRunPolicy
			kvvm := makeKVVM(vm)
			kvvm.Spec.RunStrategy = ptr.To(prevRunStrategy)
			vm.Spec.RunPolicy = runPolicy

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, makeVMIP(), makeVMClass())

			syncKVVM := NewSyncKvvmHandler(nil, fakeClient, recorder, featuregates.Default(), vmservice.NewMigrationVolumesService(fakeClient, MakeKVVMFromVMSpec, 10*time.Second))
			_, err := syncKVVM.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			_, err = NewSyncPowerStateHandler(fakeClient, recorder).Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			newKVVM := &virtv1.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newKVVM)).To(Succeed())
			started := *newKVVM.Spec.RunStrategy == virtv1.RunStrategyAlways ||
				slices.ContainsFunc(newKVVM.Status.StateChangeRequests, func(r virtv1.VirtualMachineStateChangeRequest) bool {
					return r.Action == virtv1.StartRequest
				})
			Expect(started).To(Equal(expectStart))
		},
		Entry("AlwaysOff to AlwaysOnUnlessStoppedManually starts", v1alpha2.AlwaysOffPolicy, virtv1.RunStrategyManual, v1alpha2.AlwaysOnUnlessStoppedManually, true),
		Entry("Manual to AlwaysOnUnlessStoppedManually keeps stopped", v1alpha2.ManualPolicy, virtv1.RunStrategyManual, v1alpha2.AlwaysOnUnlessStoppedManually, false),
		Entry("AlwaysOff to AlwaysOn starts", v1alpha2.AlwaysOffPolicy, virtv1.RunStrategyManual, v1alpha2.AlwaysOnPolicy, true),
		Entry("Manual to AlwaysOn starts", v1alpha2.ManualPolicy, virtv1.RunStrategyManual, v1alpha2.AlwaysOnPolicy, true),
		Entry("AlwaysOnUnlessStoppedManually to AlwaysOn starts", v1alpha2.AlwaysOnUnlessStoppedManually, virtv1.RunStrategyManual, v1alpha2.AlwaysOnPolicy, true),
		Entry("AlwaysOff to Manual keeps stopped", v1alpha2.AlwaysOffPolicy, virtv1.RunStrategyManual, v1alpha2.ManualPolicy, false),
		Entry("AlwaysOnUnlessStoppedManually to Manual keeps stopped", v1alpha2.AlwaysOnUnlessStoppedManually, virtv1.RunStrategyManual, v1alpha2.ManualPolicy, false),
		Entry("Manual to AlwaysOff keeps stopped", v1alpha2.ManualPolicy, virtv1.RunStrategyManual, v1alpha2.AlwaysOffPolicy, false),
		Entry("AlwaysOnUnlessStoppedManually to AlwaysOff keeps stopped", v1alpha2.AlwaysOnUnlessStoppedManually, virtv1.RunStrategyManual, v1alpha2.AlwaysOffPolicy, false),
		Entry("AlwaysOnUnlessStoppedManually with a pending start to AlwaysOff cancels the start", v1alpha2.AlwaysOnUnlessStoppedManually, virtv1.RunStrategyAlways, v1alpha2.AlwaysOffPolicy, false),
		Entry("AlwaysOnUnlessStoppedManually with a pending start to Manual cancels the start", v1alpha2.AlwaysOnUnlessStoppedManually, virtv1.RunStrategyAlways, v1alpha2.ManualPolicy, false),
	)

	It("starts the VM switched from AlwaysOff to AlwaysOnUnlessStoppedManually while its instance is being deleted", func() {
		vm := makeVM(v1alpha2.MachineStopped)
		vm.Spec.RunPolicy = v1alpha2.AlwaysOffPolicy
		kvvm := makeKVVM(vm)
		kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyManual)
		vm.Spec.RunPolicy = v1alpha2.AlwaysOnUnlessStoppedManually

		kvvmi := makeKVVMI()
		kvvmi.Status.Phase = virtv1.Succeeded
		kvvmi.DeletionTimestamp = ptr.To(metav1.Now())
		kvvmi.Finalizers = []string{"test"}

		fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, makeVMIP(), makeVMClass())

		reconcile := func() {
			syncKVVM := NewSyncKvvmHandler(nil, fakeClient, recorder, featuregates.Default(), vmservice.NewMigrationVolumesService(fakeClient, MakeKVVMFromVMSpec, 10*time.Second))
			_, err := syncKVVM.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			_, err = NewSyncPowerStateHandler(fakeClient, recorder).Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
		}
		reconcile()

		newKVVM := &virtv1.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newKVVM)).To(Succeed())
		Expect(newKVVM.Spec.RunStrategy).To(HaveValue(Equal(virtv1.RunStrategyManual)), "the sync waits for the instance to be gone")

		newKVVM.ResourceVersion = ""
		fakeClient, reconcileObj, vmState = setupEnvironment(vm, newKVVM, makeVMIP(), makeVMClass())
		reconcile()

		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newKVVM)).To(Succeed())
		Expect(newKVVM.Spec.RunStrategy).To(HaveValue(Equal(virtv1.RunStrategyAlways)))
	})

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

	DescribeTable("CPU features kept in the template for the running instance",
		func(restartRequested bool, expected []virtv1.CPUFeature) {
			vm := makeVM(v1alpha2.MachineRunning)
			kvvm := makeKVVM(vm)
			kvvm.Spec.Template.Spec.Domain.CPU = &virtv1.CPU{
				Model:    virtv1.CPUModeHostModel,
				Features: []virtv1.CPUFeature{{Name: "arch-capabilities", Policy: "require"}},
			}
			if restartRequested {
				kvvm.Status.StateChangeRequests = []virtv1.VirtualMachineStateChangeRequest{
					{Action: virtv1.StopRequest}, {Action: virtv1.StartRequest},
				}
			}
			kvvmi := makeKVVMI()
			kvvmi.Status.Phase = virtv1.Running

			fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, makeVMIP(), makeVMClass())
			reconcile()

			newKVVM := &virtv1.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newKVVM)).To(Succeed())
			Expect(newKVVM.Spec.Template.Spec.Domain.CPU.Features).To(Equal(expected))
		},
		Entry("give way to those of the class before a restart", true, nil),
		Entry("stay while no restart is requested", false, []virtv1.CPUFeature{{Name: "arch-capabilities", Policy: "require"}}),
	)

	It("should require restart when GPU devices change on a running VM", func() {
		ip := makeVMIP()
		vmClass := makeVMClass()

		vm := makeVM(v1alpha2.MachineRunning)
		vm.Spec.GPUs = []v1alpha2.GPUDeviceSpec{{GPUClassName: "nvidia-h100"}}
		kvvm := makeKVVM(vm)
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
				GPUs: []v1alpha2.GPUDeviceSpec{{GPUClassName: "nvidia-a100"}},
			},
		})).To(Succeed())
		kvvm.SetGroupVersionKind(virtv1.VirtualMachineGroupVersionKind)
		kvvmi := makeKVVMI()

		fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass)

		reconcile()

		newVM := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
		awaitCond, awaitExists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
		Expect(awaitExists).To(BeTrue())
		Expect(awaitCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(newVM.Status.RestartAwaitingChanges).NotTo(BeEmpty())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.Template.Spec.Domain.Devices.GPUs).To(BeEmpty())
	})

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
		Entry("should present on cpu.coreFraction change by the user", v1alpha2.MachineRunning, newFeatureGateEnableResourceInPlaceResize(), mutateCoreFraction("10%", "50%", ""), metav1.ConditionTrue, true),
		Entry("should not present on cpu.coreFraction step of the autoscaler", v1alpha2.MachineRunning, newFeatureGateEnableResourceInPlaceResize(), mutateCoreFraction("10%", string(v1alpha2.CoreFractionAuto), "50%"), metav1.ConditionUnknown, false),
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
			"should present on cpu.coreFraction change by the user when quota is insufficient during migration",
			newFeatureGate(featuregates.HotplugCPUAndMemoryWithInPlaceResize, featuregates.HotplugCPUWithLiveMigration),
			mutateCoreFraction("10%", "50%", ""),
			newResourceQuota(resource.MustParse("1"), resource.MustParse("32Gi"), resource.MustParse("500m"), resource.MustParse("2Gi")),
			metav1.ConditionTrue,
			true,
			[]string{`project quota "project-quota" has insufficient requests.cpu`},
		),
		Entry(
			"should not present on cpu.coreFraction step of the autoscaler when quota is insufficient during migration",
			newFeatureGate(featuregates.HotplugCPUAndMemoryWithInPlaceResize, featuregates.HotplugCPUWithLiveMigration),
			mutateCoreFraction("10%", string(v1alpha2.CoreFractionAuto), "50%"),
			newResourceQuota(resource.MustParse("1"), resource.MustParse("32Gi"), resource.MustParse("500m"), resource.MustParse("2Gi")),
			metav1.ConditionUnknown,
			false,
			nil,
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

	DescribeTable("ConfigurationApplied Condition for in-place resize",
		func(featureGate featuregate.FeatureGate, kvvmi *virtv1.VirtualMachineInstance, expectedMessage string) {
			ip := makeVMIP()
			vmClass := makeVMClass()
			vm := makeVM(v1alpha2.MachineRunning)
			kvvm := makeKVVM(vm)
			fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass)
			featureGates = featureGate
			reconcile()
			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())
			confAppliedCond, confAppliedExists := conditions.GetCondition(vmcondition.TypeConfigurationApplied, newVM.Status.Conditions)
			Expect(confAppliedExists).To(BeTrue())
			Expect(confAppliedCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(confAppliedCond.Reason).To(Equal(vmcondition.ReasonConfigurationNotApplied.String()))
			Expect(confAppliedCond.Message).To(Equal(expectedMessage))
			_, awaitingRestart := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
			Expect(awaitingRestart).To(BeFalse())
		},
		Entry(
			"cpu hotplug pending",
			newFeatureGateEnableResourceInPlaceResize(),
			makeResizingKVVMI(virtv1.VirtualMachineInstanceReasonPodResizePending, "Waiting for kubelet", virtv1.VirtualMachineInstanceVCPUChange),
			"The CPU changes are being applied in place.",
		),
		Entry(
			"memory hotplug in progress",
			newFeatureGateEnableResourceInPlaceResize(),
			makeResizingKVVMI(virtv1.VirtualMachineInstanceReasonPodResizeInProgress, "Resizing pod resources", virtv1.VirtualMachineInstanceMemoryChange),
			"The memory changes are being applied in place.",
		),
		Entry(
			"resize completed",
			newFeatureGateEnableResourceInPlaceResize(),
			makeResizingKVVMI(virtv1.VirtualMachineInstanceReasonPodResizeCompleted, "Completed", virtv1.VirtualMachineInstanceVCPUChange),
			"The CPU changes are being applied to the virtual machine.",
		),
		// An unknown reason is reported as progress; the raw text goes to the events.
		Entry(
			"unexpected resize reason",
			newFeatureGateEnableResourceInPlaceResize(),
			makeResizingKVVMI("UnexpectedReason", "unexpected", virtv1.VirtualMachineInstanceVCPUChange),
			"The CPU changes are being applied in place.",
		),
		Entry(
			"resize is not confirmed by the runtime",
			newFeatureGateEnableResourceInPlaceResize(),
			makeUnconfirmedResizeKVVMI(virtv1.VirtualMachineInstanceVCPUChange),
			"The CPU changes are accepted; waiting for the node to confirm the in-place change.",
		),
	)

	// The in-place marker outlives the resize once the changes go through a live migration;
	// the condition reports on the pending changes, not the marker.
	It("leaves ConfigurationApplied alone for an in-place marker without pending changes", func() {
		ip := makeVMIP()
		vmClass := makeVMClass()
		vm := makeVM(v1alpha2.MachineRunning)
		kvvm := makeKVVM(vm)
		kvvmi := makeResizingKVVMI("UnexpectedReason", "unexpected", "")
		fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass)
		featureGates = newFeatureGateEnableResourceInPlaceResize()
		reconcile()

		newVM := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
		_, exists := conditions.GetCondition(vmcondition.TypeConfigurationApplied, newVM.Status.Conditions)
		Expect(exists).To(BeFalse())
	})

	Describe("ConfigurationApplied Condition for hotplug via live migration", func() {
		const (
			nodeName = "test-node"
			podUID   = types.UID("virt-launcher-test-uid")
		)

		var events []string

		BeforeEach(func() {
			events = nil
			recorder.EventFunc = func(_ client.Object, eventType, reason, message string) {
				events = append(events, eventType+"/"+reason+": "+message)
			}
		})

		// makeHotplugVM returns a running VM with CPU and memory raised in the spec only.
		makeHotplugVM := func() *v1alpha2.VirtualMachine {
			vm := makeVM(v1alpha2.MachineRunning)
			vm.Spec.CPU.Cores = 4
			vm.Spec.CPU.CoreFraction = "100%"
			vm.Spec.Memory.Size = resource.MustParse("4Gi")
			vm.Status.Resources.CPU.Cores = 2
			vm.Status.Resources.Memory.Size = resource.MustParse("2Gi")
			return vm
		}

		makePendingKVVMI := func(conditionTypes ...virtv1.VirtualMachineInstanceConditionType) *virtv1.VirtualMachineInstance {
			kvvmi := makeKVVMI()
			kvvmi.UID = types.UID("kvvmi-test-uid")
			kvvmi.Status.NodeName = nodeName
			kvvmi.Status.ActivePods = map[types.UID]string{podUID: nodeName}
			for _, conditionType := range conditionTypes {
				kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
					Type:   conditionType,
					Status: corev1.ConditionTrue,
				})
			}
			return kvvmi
		}

		// markInplaceRefused marks the resize as rejected and returns the launcher pod with the reason.
		markInplaceRefused := func(kvvmi *virtv1.VirtualMachineInstance, podReason, podMessage string) *corev1.Pod {
			kvvmi.Annotations = map[string]string{virtv1.VirtualMachineInstanceInPlaceResizeInProgressAnn: "true"}
			kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
				Type:    virtv1.VirtualMachineInstancePodResourceResizeInProgress,
				Status:  corev1.ConditionTrue,
				Reason:  virtv1.VirtualMachineInstanceReasonPodResizePending,
				Message: podMessage,
			})
			return &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "virt-launcher-test",
					Namespace: namespace,
					UID:       podUID,
					Labels: map[string]string{
						virtv1.AppLabel:       "virt-launcher",
						virtv1.CreatedByLabel: string(kvvmi.UID),
					},
				},
				Spec: corev1.PodSpec{NodeName: nodeName},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{{
						Type:    corev1.PodResizePending,
						Status:  corev1.ConditionTrue,
						Reason:  podReason,
						Message: podMessage,
					}},
				},
			}
		}

		// makeHotplugVMOP returns a migration created for the configuration the KVVMI asks for. A
		// finished one ends a second after it is created.
		makeHotplugVMOP := func(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, name string, phase v1alpha2.VMOPPhase, completedMessage string, created time.Time) *v1alpha2.VirtualMachineOperation {
			vmop := &v1alpha2.VirtualMachineOperation{
				ObjectMeta: metav1.ObjectMeta{
					Name:              name,
					Namespace:         namespace,
					CreationTimestamp: metav1.NewTime(created),
					Annotations: map[string]string{
						annotations.AnnVMOPWorkloadUpdate: "true",
						hotplugmigration.Annotation:       hotplugmigration.Key(kvvmi),
					},
				},
				Spec: v1alpha2.VirtualMachineOperationSpec{
					Type:           v1alpha2.VMOPTypeEvict,
					VirtualMachine: vm.Name,
				},
				Status: v1alpha2.VirtualMachineOperationStatus{Phase: phase},
			}
			if completedMessage != "" || phase == v1alpha2.VMOPPhaseCompleted || phase == v1alpha2.VMOPPhaseFailed {
				vmop.Status.Conditions = []metav1.Condition{{
					Type:    vmopcondition.TypeCompleted.String(),
					Status:  metav1.ConditionFalse,
					Reason:  vmopcondition.ReasonOperationFailed.String(),
					Message: completedMessage,
				}}
				if phase == v1alpha2.VMOPPhaseCompleted || phase == v1alpha2.VMOPPhaseFailed {
					vmop.Status.Conditions[0].LastTransitionTime = metav1.NewTime(created.Add(time.Second))
				}
			}
			return vmop
		}

		makeLauncherPod := func(kvvmi *virtv1.VirtualMachineInstance, created time.Time) *corev1.Pod {
			return &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "virt-launcher-test",
					Namespace:         namespace,
					UID:               podUID,
					CreationTimestamp: metav1.NewTime(created),
					Labels: map[string]string{
						virtv1.AppLabel:                "virt-launcher",
						virtv1.CreatedByLabel:          string(kvvmi.UID),
						virtv1.VirtualMachineNameLabel: name,
					},
				},
				Spec:   corev1.PodSpec{NodeName: nodeName},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			}
		}

		type expectation struct {
			message             string
			restartRequired     bool
			restartChangesPaths []string
			warning             bool
		}

		run := func(vm *v1alpha2.VirtualMachine, gate featuregate.FeatureGate, objs ...client.Object) *v1alpha2.VirtualMachine {
			kvvm := makeKVVM(vm)
			objs = append(objs, kvvm, makeVMIP(), makeVMClass())
			fakeClient, reconcileObj, vmState = setupEnvironment(vm, objs...)
			featureGates = gate
			reconcile()
			newVM := &v1alpha2.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
			return newVM
		}

		expect := func(newVM *v1alpha2.VirtualMachine, e expectation) {
			GinkgoHelper()
			confApplied, exists := conditions.GetCondition(vmcondition.TypeConfigurationApplied, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(confApplied.Status).To(Equal(metav1.ConditionFalse))
			Expect(confApplied.Message).To(Equal(e.message))

			awaitingRestart, exists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
			Expect(exists).To(Equal(e.restartRequired))
			if e.restartRequired {
				Expect(awaitingRestart.Reason).To(Equal(vmcondition.ReasonChangesPendingRestart.String()))
				Expect(awaitingRestart.Message).To(Equal(e.message))
			}

			var paths []string
			for _, change := range newVM.Status.RestartAwaitingChanges {
				var fields map[string]interface{}
				Expect(json.Unmarshal(change.Raw, &fields)).To(Succeed())
				paths = append(paths, fields["path"].(string))
			}
			Expect(paths).To(ConsistOf(e.restartChangesPaths))

			eventType := corev1.EventTypeNormal
			if e.warning {
				eventType = corev1.EventTypeWarning
			}
			Expect(events).To(ContainElement(HavePrefix(eventType + "/" + v1alpha2.ReasonVMCPUResizing + ": " + e.message)))
		}

		It("announces the live migration while no migration exists yet", func() {
			vm := makeHotplugVM()
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange))
			expect(newVM, expectation{message: "The CPU changes will be applied by live migration."})
		})

		It("names both resources when both are pending", func() {
			vm := makeHotplugVM()
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange, virtv1.VirtualMachineInstanceMemoryChange))
			confApplied, _ := conditions.GetCondition(vmcondition.TypeConfigurationApplied, newVM.Status.Conditions)
			Expect(confApplied.Message).To(Equal("The CPU and memory changes will be applied by live migration."))
			Expect(events).To(ContainElement(HavePrefix(corev1.EventTypeNormal + "/" + v1alpha2.ReasonVMCPUAndMemoryResizing + ": ")))
		})

		It("explains the refused in-place change and the migration that follows", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
			pod := markInplaceRefused(kvvmi, string(corev1.PodReasonInfeasible), `Resize is infeasible for Guaranteed Pods alongside CPU Manager static policy`)
			newVM := run(vm, newFeatureGate(featuregates.HotplugCPUAndMemoryWithInPlaceResize, featuregates.HotplugCPUWithLiveMigration, featuregates.HotplugMemoryWithLiveMigration), kvvmi, pod)
			expect(newVM, expectation{
				message: "The node cannot apply the CPU changes in place to a virtual machine with coreFraction 100%. The CPU changes will be applied by live migration.",
			})
			Expect(events).To(ContainElement(ContainSubstring("The node reports: Resize is infeasible for Guaranteed Pods alongside CPU Manager static policy")))
		})

		It("asks for a restart when the refused in-place change cannot go through a migration", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
			pod := markInplaceRefused(kvvmi, string(corev1.PodReasonInfeasible), "infeasible")
			newVM := run(vm, newFeatureGateEnableResourceInPlaceResize(), kvvmi, pod)
			expect(newVM, expectation{
				message:             "The node cannot apply the CPU changes in place to a virtual machine with coreFraction 100%. Live migration to apply the CPU changes is disabled in the module settings. Restart the virtual machine to apply the changes.",
				restartRequired:     true,
				restartChangesPaths: []string{"cpu.cores"},
				warning:             true,
			})
		})

		It("passes on why the migration has not started", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
			vmop := makeHotplugVMOP(vm, kvvmi, "hotplug-resources-1", v1alpha2.VMOPPhasePending, "Project quota exceeded", time.Now())
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), kvvmi, vmop)
			expect(newVM, expectation{
				message: "The CPU changes are being applied by live migration. The live migration has not started yet: Project quota exceeded.",
			})
		})

		It("reports a running migration", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
			vmop := makeHotplugVMOP(vm, kvvmi, "hotplug-resources-1", v1alpha2.VMOPPhaseInProgress, "", time.Now())
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), kvvmi, vmop)
			expect(newVM, expectation{message: "The CPU changes are being applied by live migration."})
		})

		It("warns about a failed migration and tells when the next attempt is", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
			created := time.Now().Truncate(time.Second)
			vmop := makeHotplugVMOP(vm, kvvmi, "hotplug-resources-1", v1alpha2.VMOPPhaseFailed, "The migration target is unschedulable.", created)
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), kvvmi, vmop)
			expect(newVM, expectation{
				message: "Live migration to apply the CPU changes failed: The migration target is unschedulable; the next attempt is at " +
					created.Add(time.Second+5*time.Minute).UTC().Format(time.RFC3339) + ".",
				warning: true,
			})
		})

		// failedMigrations returns the last allowed number of failed migrations, finished a minute apart.
		failedMigrations := func(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) []client.Object {
			now := time.Now()
			var vmops []client.Object
			for i := range hotplugmigration.MaxAttempts {
				message := "earlier"
				if i == hotplugmigration.MaxAttempts-1 {
					message = "The migration target is unschedulable."
				}
				vmops = append(vmops, makeHotplugVMOP(vm, kvvmi, fmt.Sprintf("hotplug-resources-%d", i), v1alpha2.VMOPPhaseFailed, message, now.Add(time.Duration(i-hotplugmigration.MaxAttempts)*time.Minute)))
			}
			return vmops
		}

		It("asks for a restart once the migrations of a change made by the user have failed", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange, virtv1.VirtualMachineInstanceMemoryChange)
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), append(failedMigrations(vm, kvvmi), kvvmi)...)
			confApplied, _ := conditions.GetCondition(vmcondition.TypeConfigurationApplied, newVM.Status.Conditions)
			Expect(confApplied.Message).To(Equal("Live migration to apply the CPU and memory changes failed 4 times: The migration target is unschedulable. Restart the virtual machine to apply the changes."))
			awaitingRestart, exists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(awaitingRestart.Reason).To(Equal(vmcondition.ReasonChangesPendingRestart.String()))
			Expect(newVM.Status.RestartAwaitingChanges).To(HaveLen(2))
			Expect(events).To(ContainElement(HavePrefix(corev1.EventTypeWarning + "/" + v1alpha2.ReasonVMCPUAndMemoryResizing + ": ")))
		})

		It("migrates once more when a completed migration has not applied a change made by the user", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
			podCreated := time.Now().Add(-10 * time.Minute)
			produced := makeHotplugVMOP(vm, kvvmi, "hotplug-resources-1", v1alpha2.VMOPPhaseCompleted, "", podCreated.Add(-time.Second))
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), kvvmi, produced, makeLauncherPod(kvvmi, podCreated))
			expect(newVM, expectation{message: "The CPU changes will be applied by live migration."})
		})

		It("asks for a restart when the repeated migration has not applied a change made by the user", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
			metav1.SetMetaDataAnnotation(&kvvmi.ObjectMeta, hotplugmigration.Annotation, string(podUID)+"/"+hotplugmigration.Key(kvvmi)+"/retried")
			podCreated := time.Now().Add(-10 * time.Minute)
			produced := makeHotplugVMOP(vm, kvvmi, "hotplug-resources-1", v1alpha2.VMOPPhaseCompleted, "", podCreated.Add(-time.Second))
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), kvvmi, produced, makeLauncherPod(kvvmi, podCreated))
			expect(newVM, expectation{
				message:             "The virtual machine was migrated to apply the CPU changes, but they have not been applied. Restart the virtual machine to apply the changes.",
				restartRequired:     true,
				restartChangesPaths: []string{"cpu.cores"},
				warning:             true,
			})
		})

		It("waits for the runtime right after a completed migration", func() {
			vm := makeHotplugVM()
			kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
			podCreated := time.Now().Add(-10 * time.Second)
			produced := makeHotplugVMOP(vm, kvvmi, "hotplug-resources-1", v1alpha2.VMOPPhaseCompleted, "", podCreated.Add(-time.Second))
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), kvvmi, produced, makeLauncherPod(kvvmi, podCreated))
			expect(newVM, expectation{message: "The CPU changes are being applied to the virtual machine."})
		})

		It("asks for a restart when a change made by the user cannot be migrated", func() {
			vm := makeHotplugVM()
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:   vmcondition.TypeMigratable.String(),
				Status: metav1.ConditionFalse,
				Reason: string(vmcondition.ReasonHostDevicesNotMigratable),
			})
			newVM := run(vm, newFeatureGateEnableResourceHotplug(), makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange))
			expect(newVM, expectation{
				message:             "The virtual machine cannot be migrated to apply the CPU changes. Restart the virtual machine to apply the changes.",
				restartRequired:     true,
				restartChangesPaths: []string{"cpu.cores"},
				warning:             true,
			})
		})

		Context("with a newer CPU change while one is pending", func() {
			// runChanged applies the VM spec with 2 cores and 2Gi to the KVVM, then reconciles the VM
			// changed by mutate.
			runChanged := func(mutate func(vm *v1alpha2.VirtualMachine), conditionTypes ...virtv1.VirtualMachineInstanceConditionType) (*v1alpha2.VirtualMachine, *v1alpha2.VirtualMachineSpec) {
				vm := makeVM(v1alpha2.MachineRunning)
				vm.Spec.CPU.CoreFraction = "100%"
				vm.Status.Resources.CPU.Cores = vm.Spec.CPU.Cores
				vm.Status.Resources.Memory.Size = vm.Spec.Memory.Size
				kvvm := makeKVVM(vm)
				mutateCoreFraction("100%", "100%", "")(nil, vm, kvvm)
				mutate(vm)
				fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, makePendingKVVMI(conditionTypes...), makeVMIP(), makeVMClass())
				featureGates = newFeatureGateEnableResourceHotplug()
				reconcile()

				newVM := &v1alpha2.VirtualMachine{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
				newKVVM := &virtv1.VirtualMachine{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), newKVVM)).To(Succeed())
				lastApplied, err := kvbuilder.LoadLastAppliedSpec(newKVVM)
				Expect(err).NotTo(HaveOccurred())
				return newVM, lastApplied
			}

			It("holds the newer CPU change until the pending one is applied", func() {
				newVM, lastApplied := runChanged(func(vm *v1alpha2.VirtualMachine) { vm.Spec.CPU.Cores = 3 }, virtv1.VirtualMachineInstanceVCPUChange)
				Expect(lastApplied.CPU.Cores).To(Equal(2), "the newer number of cores must not reach the internal virtual machine")
				confApplied, _ := conditions.GetCondition(vmcondition.TypeConfigurationApplied, newVM.Status.Conditions)
				Expect(confApplied.Message).To(HaveSuffix("The newer CPU changes wait until these are applied."))
				_, awaitingRestart := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, newVM.Status.Conditions)
				Expect(awaitingRestart).To(BeFalse())
			})

			It("still applies the other changes", func() {
				_, lastApplied := runChanged(func(vm *v1alpha2.VirtualMachine) {
					vm.Spec.CPU.Cores = 3
					vm.Spec.Memory.Size = resource.MustParse("4Gi")
				}, virtv1.VirtualMachineInstanceVCPUChange)
				Expect(lastApplied.CPU.Cores).To(Equal(2))
				Expect(lastApplied.Memory.Size.String()).To(Equal("4Gi"))
			})

			It("applies a CPU change at once when none is pending", func() {
				_, lastApplied := runChanged(func(vm *v1alpha2.VirtualMachine) { vm.Spec.CPU.Cores = 3 })
				Expect(lastApplied.CPU.Cores).To(Equal(3))
			})

			It("does not hold the CPU without the last applied spec", func() {
				// loadLastAppliedSpec returns an empty spec for a KVVM with a missing or invalid annotation.
				vm := makeVM(v1alpha2.MachineRunning)
				held, ok := holdPendingCPU(vm, &v1alpha2.VirtualMachineSpec{}, makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange))
				Expect(ok).To(BeFalse())
				Expect(held).To(BeIdenticalTo(vm))
			})

			It("applies a CPU change at once when only a memory change is pending", func() {
				_, lastApplied := runChanged(func(vm *v1alpha2.VirtualMachine) { vm.Spec.CPU.Cores = 3 }, virtv1.VirtualMachineInstanceMemoryChange)
				Expect(lastApplied.CPU.Cores).To(Equal(3))
			})
		})

		Context("for a step of the autoscaler", func() {
			// makeAutoscaledVM returns a running VM whose autoscaler asks for 50% while 10% is applied.
			makeAutoscaledVM := func() *v1alpha2.VirtualMachine {
				vm := makeVM(v1alpha2.MachineRunning)
				vm.Spec.CPU.CoreFraction = string(v1alpha2.CoreFractionAuto)
				sizingpolicy.SetRecommendedCoreFraction(vm, "50%")
				vm.Status.Resources.CPU.Cores = vm.Spec.CPU.Cores
				vm.Status.Resources.CPU.CoreFraction = "10%"
				return vm
			}

			runAutoscaled := func(vm *v1alpha2.VirtualMachine, objs ...client.Object) *v1alpha2.VirtualMachine {
				kvvm := makeKVVM(vm)
				mutateCoreFraction("50%", string(v1alpha2.CoreFractionAuto), "50%")(nil, vm, kvvm)
				objs = append(objs, kvvm, makeVMIP(), makeVMClass())
				fakeClient, reconcileObj, vmState = setupEnvironment(vm, objs...)
				featureGates = newFeatureGateEnableResourceHotplug()
				reconcile()
				newVM := &v1alpha2.VirtualMachine{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
				return newVM
			}

			// runningAt sets the core fraction the launcher pod runs with.
			runningAt := func(pod *corev1.Pod, fraction string) *corev1.Pod {
				pod.Annotations = map[string]string{kvbuilder.CPUResourcesRequestsFractionAnnotation: fraction}
				return pod
			}

			It("reports the failed migrations without asking for a restart", func() {
				vm := makeAutoscaledVM()
				kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
				pod := runningAt(makeLauncherPod(kvvmi, time.Now().Add(-time.Hour)), "10")
				newVM := runAutoscaled(vm, append(failedMigrations(vm, kvvmi), kvvmi, pod)...)
				expect(newVM, expectation{
					message: "The autoscaler has selected a CPU core fraction of 50%. Live migration to apply the CPU changes failed 4 times: The migration target is unschedulable. The core fraction stays at 10%.",
					warning: true,
				})
			})

			It("does not make up the core fraction the virtual machine runs with when it is unknown", func() {
				vm := makeAutoscaledVM()
				kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
				newVM := runAutoscaled(vm, append(failedMigrations(vm, kvvmi), kvvmi)...)
				expect(newVM, expectation{
					message: "The autoscaler has selected a CPU core fraction of 50%. Live migration to apply the CPU changes failed 4 times: The migration target is unschedulable. The core fraction stays as it is.",
					warning: true,
				})
			})

			It("reports a repeated migration that has not applied the step without asking for a restart", func() {
				vm := makeAutoscaledVM()
				kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
				metav1.SetMetaDataAnnotation(&kvvmi.ObjectMeta, hotplugmigration.Annotation, string(podUID)+"/"+hotplugmigration.Key(kvvmi)+"/retried")
				podCreated := time.Now().Add(-10 * time.Minute)
				produced := makeHotplugVMOP(vm, kvvmi, "hotplug-resources-1", v1alpha2.VMOPPhaseCompleted, "", podCreated.Add(-time.Second))
				newVM := runAutoscaled(vm, kvvmi, produced, runningAt(makeLauncherPod(kvvmi, podCreated), "10"))
				expect(newVM, expectation{
					message: "The autoscaler has selected a CPU core fraction of 50%. The virtual machine was migrated to apply the CPU changes, but they have not been applied. The core fraction stays at 10%.",
					warning: true,
				})
			})

			It("leaves the step to the node when the virtual machine cannot be migrated", func() {
				vm := makeAutoscaledVM()
				vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
					Type:   vmcondition.TypeMigratable.String(),
					Status: metav1.ConditionFalse,
					Reason: string(vmcondition.ReasonHostDevicesNotMigratable),
				})
				newVM := runAutoscaled(vm, makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange))
				expect(newVM, expectation{
					message: "The autoscaler has selected a CPU core fraction of 50%. The virtual machine cannot be migrated to another node, so the CPU changes wait until its node can apply them in place.",
				})
			})

			It("does not ask for a restart when live migration is disabled in the module settings", func() {
				vm := makeAutoscaledVM()
				kvvm := makeKVVM(vm)
				mutateCoreFraction("50%", string(v1alpha2.CoreFractionAuto), "50%")(nil, vm, kvvm)
				kvvmi := makePendingKVVMI(virtv1.VirtualMachineInstanceVCPUChange)
				fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, runningAt(makeLauncherPod(kvvmi, time.Now().Add(-time.Hour)), "10"), makeVMIP(), makeVMClass())
				featureGates = newFeatureGateEnableResourceInPlaceResize()
				reconcile()
				newVM := &v1alpha2.VirtualMachine{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
				expect(newVM, expectation{
					message: "The autoscaler has selected a CPU core fraction of 50%. Live migration to apply the CPU changes is disabled in the module settings. The core fraction stays at 10%.",
					warning: true,
				})
			})
		})
	})

	It("does not expose apply-immediate block device changes in RestartAwaitingChanges while dependencies are not ready", func() {
		ip := makeVMIP()
		vmClass := makeVMClass()

		rootRef := v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "root"}
		blankRef := v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "blank-disk"}

		vm := makeVM(v1alpha2.MachineRunning)
		vm.Spec.EnableParavirtualization = ptr.To(true)
		vm.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{rootRef, blankRef}
		vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
			Type:   vmcondition.TypeBlockDevicesReady.String(),
			Status: metav1.ConditionFalse,
			Reason: "BlockDevicesNotReady",
		})

		kvvm := makeKVVM(vm)
		Expect(kvbuilder.SetLastAppliedSpec(kvvm, &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				CPU: v1alpha2.CPUSpec{
					Cores: vm.Spec.CPU.Cores,
				},
				Memory: v1alpha2.MemorySpec{
					Size: vm.Spec.Memory.Size,
				},
				VirtualMachineIPAddress:       vm.Spec.VirtualMachineIPAddress,
				RunPolicy:                     vm.Spec.RunPolicy,
				OsType:                        vm.Spec.OsType,
				VirtualMachineClassName:       vm.Spec.VirtualMachineClassName,
				EnableParavirtualization:      ptr.To(true),
				BlockDeviceRefs:               []v1alpha2.BlockDeviceSpecRef{rootRef},
				TerminationGracePeriodSeconds: vm.Spec.TerminationGracePeriodSeconds,
				Disruptions: &v1alpha2.Disruptions{
					RestartApprovalMode: vm.Spec.Disruptions.RestartApprovalMode,
				},
			},
		})).To(Succeed())

		kvvmi := makeKVVMI()
		fakeClient, reconcileObj, vmState = setupEnvironment(vm, kvvm, kvvmi, ip, vmClass)

		reconcile()

		updatedVM := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), updatedVM)).To(Succeed())
		Expect(updatedVM.Status.RestartAwaitingChanges).To(BeEmpty())
		awaitCond, awaitExists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, updatedVM.Status.Conditions)
		Expect(awaitExists).To(BeFalse(), "ApplyImmediate block device changes should not set %s, got %+v", vmcondition.TypeAwaitingRestartToApplyConfiguration, awaitCond)
	})

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

	It("upgrades only block device changes touching non-hotpluggable volumes", func() {
		rootRef := v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "root"}
		dataRef := v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "data"}
		extraRef := v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "extra"}

		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Spec.Volumes = []virtv1.Volume{
			{
				Name: kvbuilder.GenerateDiskName(rootRef.Kind, rootRef.Name),
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{Hotpluggable: false},
				},
			},
			{
				Name: kvbuilder.GenerateDiskName(dataRef.Kind, dataRef.Name),
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{Hotpluggable: true},
				},
			},
		}

		nonHotpluggableRefs := nonHotpluggableVolumeRefs(kvvmi)
		changes := vmchange.SpecChanges{}
		changes.Add(
			vmchange.FieldChange{Path: "blockDeviceRefs.0", Operation: vmchange.ChangeRemove, CurrentValue: dataRef, ActionRequired: vmchange.ActionApplyImmediate},
			vmchange.FieldChange{Path: "blockDeviceRefs.1", Operation: vmchange.ChangeAdd, DesiredValue: extraRef, ActionRequired: vmchange.ActionApplyImmediate},
			vmchange.FieldChange{Path: "blockDeviceRefs.2", Operation: vmchange.ChangeReplace, CurrentValue: rootRef, DesiredValue: dataRef, ActionRequired: vmchange.ActionApplyImmediate},
			vmchange.FieldChange{Path: "blockDeviceRefs.3", Operation: vmchange.ChangeAdd, DesiredValue: rootRef, ActionRequired: vmchange.ActionApplyImmediate},
			vmchange.FieldChange{Path: "cpu.cores", Operation: vmchange.ChangeReplace, ActionRequired: vmchange.ActionApplyImmediate},
		)

		changes.UpgradeBlockDeviceChangesToRestartMatching(func(change vmchange.FieldChange) bool {
			return blockDeviceChangeTouchesRefs(change, nonHotpluggableRefs)
		})

		Expect(changes.GetAll()[0].ActionRequired).To(Equal(vmchange.ActionApplyImmediate))
		Expect(changes.GetAll()[1].ActionRequired).To(Equal(vmchange.ActionApplyImmediate))
		Expect(changes.GetAll()[2].ActionRequired).To(Equal(vmchange.ActionRestart))
		Expect(changes.GetAll()[3].ActionRequired).To(Equal(vmchange.ActionRestart))
		Expect(changes.GetAll()[4].ActionRequired).To(Equal(vmchange.ActionApplyImmediate))
	})

	deletingKVVMI := func() *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.Phase = virtv1.Succeeded
		kvvmi.DeletionTimestamp = ptr.To(metav1.Now())
		kvvmi.Finalizers = []string{"test"}
		return kvvmi
	}

	DescribeTable("KVVM sync of a stopped VM while the previous instance is torn down",
		func(runStrategy virtv1.VirtualMachineRunStrategy, startRequested bool, kvvmi *virtv1.VirtualMachineInstance, expectSynced bool) {
			ip := makeVMIP()
			vmClass := makeVMClass()

			vm := makeVM(v1alpha2.MachineStopped)
			kvvm := makeKVVM(vm)
			kvvm.Spec.RunStrategy = ptr.To(runStrategy)
			kvvm.Status.Created = kvvmi != nil
			if startRequested {
				kvvm.Status.StateChangeRequests = []virtv1.VirtualMachineStateChangeRequest{{Action: virtv1.StartRequest}}
			}
			mutateCPUCores(3)(nil, vm, kvvm)

			objs := []client.Object{kvvm, ip, vmClass}
			if kvvmi != nil {
				objs = append(objs, kvvmi)
			}
			fakeClient, reconcileObj, vmState = setupEnvironment(vm, objs...)

			h := NewSyncKvvmHandler(nil, fakeClient, recorder, featuregates.Default(), vmservice.NewMigrationVolumesService(fakeClient, MakeKVVMFromVMSpec, 10*time.Second))
			result, err := h.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			updatedKVVM := &virtv1.VirtualMachine{}
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), updatedKVVM)).To(Succeed())
			if expectSynced {
				Expect(updatedKVVM.Spec.Template.Spec.Domain.CPU).NotTo(BeNil())
				return
			}
			Expect(updatedKVVM.Spec.Template.Spec.Domain.CPU).To(BeNil(), "KVVM must not be rewritten while the instance still exists")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		},
		Entry("postpones the sync while the instance is being deleted", virtv1.RunStrategyManual, false, deletingKVVMI(), false),
		Entry("syncs once the instance is gone", virtv1.RunStrategyManual, false, nil, true),
		Entry("syncs while a completed instance is not deleted yet", virtv1.RunStrategyManual, false, func() *virtv1.VirtualMachineInstance {
			kvvmi := newEmptyKVVMI(name, namespace)
			kvvmi.Status.Phase = virtv1.Succeeded
			return kvvmi
		}(), true),
		Entry("syncs a KVVM that starts on its own", virtv1.RunStrategyAlways, false, deletingKVVMI(), true),
		Entry("syncs a KVVM with a pending start request", virtv1.RunStrategyManual, true, deletingKVVMI(), true),
	)

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

func newFeatureGateEnableResourceInPlaceResize() featuregate.FeatureGate {
	return newFeatureGate(featuregates.HotplugCPUAndMemoryWithInPlaceResize)
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

var _ = Describe("SyncKvvmHandler.isVMStopped", func() {
	const (
		name      = "vm-stopped"
		namespace = "default"
	)

	var (
		handler *SyncKvvmHandler
		vm      *v1alpha2.VirtualMachine
		kvvm    *virtv1.VirtualMachine
		kvvmi   *virtv1.VirtualMachineInstance
	)

	BeforeEach(func() {
		handler = &SyncKvvmHandler{}
		vm = vmbuilder.NewEmpty(name, namespace)
		vm.Status.Phase = v1alpha2.MachineStopped
		kvvm = &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Status: virtv1.VirtualMachineStatus{
				PrintableStatus: virtv1.VirtualMachineStatusStopped,
			},
		}
		kvvmi = &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Status:     virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Pending},
		}
	})

	It("reports a stopped VM when no instance exists", func() {
		Expect(handler.isVMStopped(vm, kvvm, nil, nil)).To(BeTrue())
	})

	// The internal virtual machine's status is written by virt-controller and lags behind the
	// instance it describes, so a freshly created instance is routinely paired with a status
	// still saying "Stopped". Taking the stopped branch there rewrites the whole spec under a
	// live instance and makes virt-controller tear it down.
	It("does not report a stopped VM while an instance exists, even a Pending one", func() {
		Expect(handler.isVMStopped(vm, kvvm, kvvmi, nil)).To(BeFalse())
	})

	It("does not report a stopped VM while a running instance exists", func() {
		kvvmi.Status.Phase = virtv1.Running
		Expect(handler.isVMStopped(vm, kvvm, kvvmi, nil)).To(BeFalse())
	})

	// A guest-initiated reboot or shutdown leaves the instance behind in a final phase. That is
	// the only window in which a disruptive change awaiting a restart reaches the internal
	// virtual machine, so a finished instance must not be mistaken for a live one.
	DescribeTable("reports a stopped VM once the instance has finished",
		func(phase virtv1.VirtualMachineInstancePhase) {
			kvvmi.Status.Phase = phase
			Expect(handler.isVMStopped(vm, kvvm, kvvmi, nil)).To(BeTrue())
		},
		Entry("succeeded", virtv1.Succeeded),
		Entry("failed", virtv1.Failed),
	)

	It("reports a stopped VM while the instance is being deleted", func() {
		kvvmi.Status.Phase = virtv1.Running
		kvvmi.DeletionTimestamp = ptr.To(metav1.Now())
		Expect(handler.isVMStopped(vm, kvvm, kvvmi, nil)).To(BeTrue())
	})

	It("does not report a stopped VM when a finished instance still has a live pod", func() {
		kvvmi.Status.Phase = virtv1.Succeeded
		kvvm.Status.Created = true
		pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}
		Expect(handler.isVMStopped(vm, kvvm, kvvmi, pod)).To(BeFalse())
	})
})
