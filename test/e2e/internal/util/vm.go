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

package util

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

func UntilVMAgentReady(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		vm, err := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, vm.Status.Conditions)
		if agentReady.Status == metav1.ConditionTrue {
			return nil
		}

		return fmt.Errorf("vm %s is not ready", key.Name)
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func UntilVMMigrationSucceeded(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		vm, err := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		state := vm.Status.MigrationState

		if state == nil || state.EndTimestamp.IsZero() {
			return fmt.Errorf("migration is not completed")
		}

		switch state.Result {
		case v1alpha2.MigrationResultSucceeded:
			return nil
		case v1alpha2.MigrationResultFailed:
			migrating, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
			msg := fmt.Sprintf("migration failed: reason: %s, message: %s", migrating.Reason, migrating.Message)
			Fail(msg)
		}

		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func MigrateVirtualMachine(f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...vmopbuilder.Option) {
	GinkgoHelper()

	opts := []vmopbuilder.Option{
		vmopbuilder.WithGenerateName("vmop-e2e-"),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vm.Name),
	}
	opts = append(opts, options...)
	vmop := vmopbuilder.New(opts...)

	err := f.CreateWithDeferredDeletion(context.Background(), vmop)
	Expect(err).NotTo(HaveOccurred())
}

func StartVirtualMachine(f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...vmopbuilder.Option) {
	GinkgoHelper()

	opts := []vmopbuilder.Option{
		vmopbuilder.WithGenerateName("vmop-e2e-"),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
		vmopbuilder.WithVirtualMachine(vm.Name),
	}
	opts = append(opts, options...)
	vmop := vmopbuilder.New(opts...)

	err := f.CreateWithDeferredDeletion(context.Background(), vmop)
	Expect(err).NotTo(HaveOccurred())
}

func StopVirtualMachineFromOS(f *framework.Framework, vm *v1alpha2.VirtualMachine) error {
	_, err := f.SSHCommand(vm.Name, vm.Namespace, "sudo init 0")
	if err != nil && strings.Contains(err.Error(), "unexpected EOF") {
		return nil
	}
	return err
}

func RebootVirtualMachineBySSH(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, "sudo reboot")
	Expect(err).NotTo(HaveOccurred())
}

func RebootVirtualMachineByVMOP(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	vmop := vmopbuilder.New(
		vmopbuilder.WithGenerateName("vmop-e2e-reboot-"),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestart),
		vmopbuilder.WithVirtualMachine(vm.Name),
	)
	err := f.CreateWithDeferredDeletion(context.Background(), vmop)
	Expect(err).NotTo(HaveOccurred())
}

func UntilVirtualMachineRebooted(key client.ObjectKey, previousRunningTime time.Time, timeout time.Duration) {
	Eventually(func() error {
		vm := &v1alpha2.VirtualMachine{}
		err := framework.GetClients().GenericClient().Get(context.Background(), key, vm)
		if err != nil {
			return fmt.Errorf("failed to get virtual machine: %w", err)
		}

		runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)

		if runningCondition.LastTransitionTime.Time.After(previousRunningTime) && vm.Status.Phase == v1alpha2.MachineRunning {
			return nil
		}

		return fmt.Errorf("virtual machine %s is not rebooted", key.Name)
	}, timeout, time.Second).Should(Succeed())
}

func IsVDAttached(vm *v1alpha2.VirtualMachine, vd *v1alpha2.VirtualDisk) bool {
	for _, bd := range vm.Status.BlockDeviceRefs {
		if bd.Kind == v1alpha2.DiskDevice && bd.Name == vd.Name && bd.Attached {
			return true
		}
	}
	return false
}

func IsRestartRequired(vm *v1alpha2.VirtualMachine, timeout time.Duration) bool {
	GinkgoHelper()

	if vm.Spec.Disruptions.RestartApprovalMode != v1alpha2.Manual {
		return false
	}

	Eventually(func(g Gomega) {
		err := framework.GetClients().GenericClient().Get(context.Background(), client.ObjectKeyFromObject(vm), vm)
		g.Expect(err).NotTo(HaveOccurred())
		needRestart, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
		g.Expect(needRestart.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(vm.Status.RestartAwaitingChanges).NotTo(BeNil())
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())

	return true
}
