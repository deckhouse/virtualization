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
	"github.com/deckhouse/virtualization/tests/e2e/framework"
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

	By("Wait until VM migration succeeded")
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
			Fail("migration failed")
		}

		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func MigrateVirtualMachine(vm *v1alpha2.VirtualMachine, options ...vmopbuilder.Option) {
	GinkgoHelper()
	By("Starting migrations for virtual machines")

	opts := []vmopbuilder.Option{
		vmopbuilder.WithGenerateName("vmop-e2e-"),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vm.Name),
	}
	opts = append(opts, options...)
	vmop := vmopbuilder.New(opts...)

	_, err := framework.GetClients().VirtClient().VirtualMachineOperations(vm.Namespace).Create(context.Background(), vmop, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func StopVirtualMachineFromOS(f *framework.Framework, vm *v1alpha2.VirtualMachine) error {
	By(fmt.Sprintf("Exec shutdown command for virtualmachine %s/%s", vm.Namespace, vm.Name))

	err := f.SSHCommand(vm.Name, vm.Namespace, "sudo init 0")
	if err != nil && strings.Contains(err.Error(), "unexpected EOF") {
		return nil
	}
	return err
}

func ExecStressNGInVirtualMachine(f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...framework.SSHCommandOption) {
	GinkgoHelper()

	cmd := "sudo nohup stress-ng --vm 1 --vm-bytes 100% --timeout 300s &>/dev/null &"

	By(fmt.Sprintf("Exec StressNG command for virtualmachine %s/%s", vm.Namespace, vm.Name))
	Expect(f.SSHCommand(vm.Name, vm.Namespace, cmd, options...)).To(Succeed())

	By("Wait until stress-ng loads the memory more heavily")
	time.Sleep(20 * time.Second)
}
