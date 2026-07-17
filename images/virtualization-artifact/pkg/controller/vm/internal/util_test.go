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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("isPodStartedError", func() {
	newKVVMWithSynchronized := func(reason string) *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{
			Status: virtv1.VirtualMachineStatus{
				Conditions: []virtv1.VirtualMachineCondition{
					{
						Type:   virtv1.VirtualMachineConditionType(virtv1.VirtualMachineInstanceSynchronized),
						Status: corev1.ConditionFalse,
						Reason: reason,
					},
				},
			},
		}
	}

	It("detects backend storage creation failure", func() {
		Expect(isPodStartedError(newKVVMWithSynchronized(failedBackendStorageCreateReason))).To(BeTrue())
	})

	It("detects pod creation failure", func() {
		Expect(isPodStartedError(newKVVMWithSynchronized(failedCreatePodReason))).To(BeTrue())
	})

	It("ignores unrelated synchronized reasons", func() {
		Expect(isPodStartedError(newKVVMWithSynchronized("SomethingElse"))).To(BeFalse())
	})
})

var _ = Describe("vmStartupMessage", func() {
	kvvmSynchronized := func(reason, message string) *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{
			Status: virtv1.VirtualMachineStatus{
				Conditions: []virtv1.VirtualMachineCondition{
					{
						Type:    virtv1.VirtualMachineConditionType(virtv1.VirtualMachineInstanceSynchronized),
						Status:  corev1.ConditionFalse,
						Reason:  reason,
						Message: message,
					},
				},
			},
		}
	}
	kvvmStatus := func(status virtv1.VirtualMachinePrintableStatus) *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{Status: virtv1.VirtualMachineStatus{PrintableStatus: status}}
	}

	It("explains backend storage failure and keeps the underlying detail", func() {
		Expect(vmStartupMessage(kvvmSynchronized(failedBackendStorageCreateReason, "no default storage class found"))).
			To(Equal("Cannot provision storage for the virtual machine's Secure Boot state: no default storage class found."))
	})

	It("maps a printable status to a fixed message without internal phases", func() {
		Expect(vmStartupMessage(kvvmStatus(virtv1.VirtualMachineStatusUnschedulable))).
			To(Equal("The virtual machine cannot be scheduled onto any node."))
	})

	It("falls back for an unknown cause", func() {
		Expect(vmStartupMessage(kvvmStatus(""))).To(Equal("The virtual machine failed to start."))
	})
})

var _ = Describe("getPhase", func() {
	newVM := func(runPolicy v1alpha2.RunPolicy, phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{}
		vm.Spec.RunPolicy = runPolicy
		vm.Status.Phase = phase
		return vm
	}

	newStoppedKVVM := func(runStrategy virtv1.VirtualMachineRunStrategy, anns map[string]string) *virtv1.VirtualMachine {
		kvvm := &virtv1.VirtualMachine{}
		kvvm.Annotations = anns
		kvvm.Spec.RunStrategy = ptr.To(runStrategy)
		kvvm.Status.PrintableStatus = virtv1.VirtualMachineStatusStopped
		return kvvm
	}

	Context("KVVM is stopped", func() {
		It("returns Pending while a fresh AlwaysOnUnlessStoppedManually KVVM awaits auto-start", func() {
			vm := newVM(v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.MachinePending)
			kvvm := newStoppedKVVM(virtv1.RunStrategyAlways, nil)
			Expect(getPhase(vm, kvvm)).To(Equal(v1alpha2.MachinePending))
		})

		It("returns Pending for AlwaysOn policy regardless of run strategy", func() {
			vm := newVM(v1alpha2.AlwaysOnPolicy, v1alpha2.MachinePending)
			kvvm := newStoppedKVVM(virtv1.RunStrategyManual, nil)
			Expect(getPhase(vm, kvvm)).To(Equal(v1alpha2.MachinePending))
		})

		It("returns Pending when a manual run-strategy KVVM has a start request", func() {
			vm := newVM(v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.MachinePending)
			kvvm := newStoppedKVVM(virtv1.RunStrategyManual, map[string]string{annotations.AnnVMStartRequested: "true"})
			Expect(getPhase(vm, kvvm)).To(Equal(v1alpha2.MachinePending))
		})

		It("returns Pending when a manual run-strategy KVVM has a restart request", func() {
			vm := newVM(v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.MachinePending)
			kvvm := newStoppedKVVM(virtv1.RunStrategyManual, map[string]string{annotations.AnnVMRestartRequested: "true"})
			Expect(getPhase(vm, kvvm)).To(Equal(v1alpha2.MachinePending))
		})

		It("returns Stopped when a KVVM is deliberately kept stopped after restore", func() {
			vm := newVM(v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.MachinePending)
			kvvm := newStoppedKVVM(virtv1.RunStrategyManual, nil)
			Expect(getPhase(vm, kvvm)).To(Equal(v1alpha2.MachineStopped))
		})

		It("returns Stopped for Manual policy", func() {
			vm := newVM(v1alpha2.ManualPolicy, v1alpha2.MachinePending)
			kvvm := newStoppedKVVM(virtv1.RunStrategyManual, nil)
			Expect(getPhase(vm, kvvm)).To(Equal(v1alpha2.MachineStopped))
		})

		It("returns Stopped when the previous phase is not Pending", func() {
			vm := newVM(v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.MachineStopping)
			kvvm := newStoppedKVVM(virtv1.RunStrategyManual, nil)
			Expect(getPhase(vm, kvvm)).To(Equal(v1alpha2.MachineStopped))
		})
	})
})
