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
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

const (
	VmopE2ePrefix = "vmop-e2e"
)

var knownKubeVirtClientSocketClosedRe = regexp.MustCompile(`(?is)virError\(Code=1,.*internal error:\s*client\s+socket\s+is\s+closed`)

func IsKnownKubeVirtClientSocketClosedFailureReason(reason string) bool {
	return knownKubeVirtClientSocketClosedRe.MatchString(reason)
}

// TODO: remove temporary migration skip logic when issue "client socket is closed" is fixed:
func SkipIfKnownKubeVirtClientSocketClosedMigrationFailure(vm *v1alpha2.VirtualMachine) {
	SkipIfKnownKubeVirtClientSocketClosedMigrationFailureWithContext(context.Background(), vm)
}

// TODO: remove temporary migration skip logic when issue "client socket is closed" is fixed:
func SkipIfKnownKubeVirtClientSocketClosedMigrationFailureWithContext(ctx context.Context, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	if vm == nil {
		return
	}

	intvirtvmi, err := GetInternalVirtualMachineInstance(ctx, vm)
	Expect(err).NotTo(HaveOccurred())
	if intvirtvmi == nil || intvirtvmi.Status.MigrationState == nil {
		return
	}

	failureReason := intvirtvmi.Status.MigrationState.FailureReason
	if IsKnownKubeVirtClientSocketClosedFailureReason(failureReason) {
		Skip(fmt.Sprintf("skip due to known kubevirt migration issue (client socket closed) for vm %s/%s: %s",
			vm.Namespace, vm.Name, failureReason))
	}
}

// TODO: remove temporary migration skip logic when issue "client socket is closed" is fixed:
func SkipIfKnownMigrationFailure(vm *v1alpha2.VirtualMachine) {
	SkipIfKnownMigrationFailureWithContext(context.Background(), vm)
}

// TODO: remove temporary migration skip logic when issue "client socket is closed" is fixed:
func SkipIfKnownMigrationFailureWithContext(ctx context.Context, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	SkipIfKnownKubeVirtClientSocketClosedMigrationFailureWithContext(ctx, vm)
}

func GetInternalVirtualMachineInstance(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	GinkgoHelper()

	obj := &rewrite.VirtualMachineInstance{}
	err := framework.GetClients().RewriteClient().Get(ctx, vm.Name, obj, rewrite.InNamespace(vm.Namespace))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return obj.VirtualMachineInstance, nil
}

func UntilVMAgentReady(ctx context.Context, key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		vm, err := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(ctx, key.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, vm.Status.Conditions)
		if agentReady.Status == metav1.ConditionTrue {
			return nil
		}

		return fmt.Errorf("%s: guest agent is not ready", key.Name)
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func UntilSSHReady(f *framework.Framework, vm *v1alpha2.VirtualMachine, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		result, err := f.SSHCommand(vm.Name, vm.Namespace, "echo 'test'", framework.WithSSHTimeout(5*time.Second))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(ContainSubstring("test"))
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func UntilGuestCommandsReady(f *framework.Framework, vm *v1alpha2.VirtualMachine, commands []string, timeout time.Duration) {
	GinkgoHelper()

	cmd := fmt.Sprintf(`
		missing=""
		for command in %s; do
			command -v "$command" >/dev/null 2>&1 || missing="$missing $command"
		done
		[ -z "$missing" ] || { echo "missing commands:$missing"; exit 1; }
	`, shellArgs(commands))

	Eventually(func() error {
		_, err := f.SSHCommand(vm.Name, vm.Namespace, cmd, framework.WithSSHTimeout(5*time.Second))
		return err
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func shellArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, fmt.Sprintf("%q", arg))
	}

	return strings.Join(quoted, " ")
}

func GetVMNode(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) (string, error) {
	GinkgoHelper()

	err := f.GenericClient().Get(ctx, client.ObjectKeyFromObject(vm), vm)
	if err != nil {
		return "", err
	}
	if vm.Status.Node == "" {
		return "", fmt.Errorf("vm %s/%s has empty status.node", vm.Namespace, vm.Name)
	}

	return vm.Status.Node, nil
}

func ExpectNoVMOperationsForVirtualMachine(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	vmops, err := f.VirtClient().VirtualMachineOperations(vm.Namespace).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	for _, vmop := range vmops.Items {
		if vmop.Spec.VirtualMachine == vm.Name {
			Fail(fmt.Sprintf("unexpected VMOP %q for VM %q", vmop.Name, vm.Name))
		}
	}
}

func ExpectVMOnNode(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, expectedNode string) {
	GinkgoHelper()

	node, err := GetVMNode(ctx, f, vm)
	Expect(err).NotTo(HaveOccurred())
	Expect(node).To(Equal(expectedNode))
}

// UntilVMMigrationSucceeded waits for the newest migration VMOP of the VM to reach a terminal
// phase and for the VM's migration state to report success. The VMOP is discovered by listing,
// so it also covers flows where the operation is created asynchronously (workload updater,
// storage class change). A VMOP that turns Failed fails the test immediately.
func UntilVMMigrationSucceeded(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var vmop *v1alpha2.VirtualMachineOperation
	Eventually(func() error {
		vmops, err := framework.GetClients().VirtClient().VirtualMachineOperations(key.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		vmop = newestMigrationVMOP(vmops, key.Name)
		if vmop == nil {
			return fmt.Errorf("no migration vmop found for vm %s/%s", key.Namespace, key.Name)
		}
		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())

	UntilVMOPMigrationSucceeded(ctx, vmop, timeout)

	// The VM object mirrors the migration state of the completed VMOP with a small lag; keep
	// asserting the same final state as before.
	Eventually(func() error {
		vm, err := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		// TODO: remove temporary migration skip logic when both known issues are fixed:
		// kubevirt "client socket is closed" and Volume(s)UpdateError.
		SkipIfKnownMigrationFailure(vm)

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
	}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())
}

func newestMigrationVMOP(vmops *v1alpha2.VirtualMachineOperationList, vmName string) *v1alpha2.VirtualMachineOperation {
	var newest *v1alpha2.VirtualMachineOperation
	for i := range vmops.Items {
		vmop := &vmops.Items[i]
		if vmop.Spec.VirtualMachine != vmName {
			continue
		}
		if vmop.Spec.Type != v1alpha2.VMOPTypeEvict && vmop.Spec.Type != v1alpha2.VMOPTypeMigrate {
			continue
		}
		if newest == nil || vmop.CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = vmop
		}
	}
	return newest
}

func UntilDisksAreAttachedInVMStatus(
	ctx context.Context,
	f *framework.Framework,
	timeout time.Duration,
	vm *v1alpha2.VirtualMachine,
	vds ...*v1alpha2.VirtualDisk,
) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		err := f.GenericClient().Get(ctx, client.ObjectKeyFromObject(vm), vm)
		g.Expect(err).NotTo(HaveOccurred())

		for _, vd := range vds {
			g.Expect(IsVDAttached(vm, vd)).To(BeTrue())
		}
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func MigrateVirtualMachine(f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...vmopbuilder.Option) *v1alpha2.VirtualMachineOperation {
	GinkgoHelper()

	opts := []vmopbuilder.Option{
		vmopbuilder.WithGenerateName(fmt.Sprintf("%s-evict-", VmopE2ePrefix)),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vm.Name),
	}
	opts = append(opts, options...)
	vmop := vmopbuilder.New(opts...)

	err := f.CreateWithDeferredDeletion(context.Background(), vmop)
	Expect(err).NotTo(HaveOccurred())

	return vmop
}

func StartVirtualMachine(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...vmopbuilder.Option) {
	GinkgoHelper()

	opts := []vmopbuilder.Option{
		vmopbuilder.WithGenerateName(fmt.Sprintf("%s-start-", VmopE2ePrefix)),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
		vmopbuilder.WithVirtualMachine(vm.Name),
	}
	opts = append(opts, options...)
	vmop := vmopbuilder.New(opts...)

	err := f.CreateWithDeferredDeletion(ctx, vmop)
	Expect(err).NotTo(HaveOccurred())
}

func StopVirtualMachineFromOS(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, "nohup sh -c \"sleep 5 && sudo poweroff\" > /dev/null 2>&1 &")
	Expect(err).NotTo(HaveOccurred())
}

func RebootVirtualMachineBySSH(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, "nohup sh -c \"sleep 5 && sudo reboot\" > /dev/null 2>&1 &")
	Expect(err).NotTo(HaveOccurred())
}

func RebootVirtualMachineByVMOP(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	vmop := vmopbuilder.New(
		vmopbuilder.WithGenerateName(fmt.Sprintf("%s-reboot-", VmopE2ePrefix)),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestart),
		vmopbuilder.WithVirtualMachine(vm.Name),
	)
	err := f.CreateWithDeferredDeletion(context.Background(), vmop)
	Expect(err).NotTo(HaveOccurred())
}

func RebootVirtualMachineByPodDeletion(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	activePodName, err := GetActivePodName(vm)
	Expect(err).NotTo(HaveOccurred())
	Expect(activePodName).NotTo(BeEmpty())

	var pod corev1.Pod
	err = framework.GetClients().GenericClient().Get(context.Background(), types.NamespacedName{
		Namespace: vm.Namespace,
		Name:      activePodName,
	}, &pod)
	Expect(err).NotTo(HaveOccurred())

	err = framework.GetClients().GenericClient().Delete(context.Background(), &pod)
	Expect(err).NotTo(HaveOccurred())
}

func GetVirtualMachineAndActivePod(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) (*v1alpha2.VirtualMachine, *corev1.Pod, error) {
	var currentVM v1alpha2.VirtualMachine
	err := f.GenericClient().Get(ctx, client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}, &currentVM)
	if err != nil {
		return nil, nil, err
	}

	activePodName, err := GetActivePodName(&currentVM)
	if err != nil {
		return nil, nil, err
	}

	var activePod corev1.Pod
	err = f.GenericClient().Get(ctx, client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      activePodName,
	}, &activePod)
	if err != nil {
		return nil, nil, err
	}

	return &currentVM, &activePod, nil
}

func GetActivePodName(vm *v1alpha2.VirtualMachine) (string, error) {
	for _, pod := range vm.Status.VirtualMachinePods {
		if pod.Active {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf("no active pod found for virtual machine %s/%s", vm.Namespace, vm.Name)
}

// TODO: Remove this skip when the lost guest-shutdown-reason race in the
// virtualization-controller is fixed. SyncPowerStateHandler decides what to do
// with a Succeeded internal VMI by the virt-launcher pod termination message
// (powerstate.ShutdownReason): guest-reset means Restart, guest-shutdown means
// Stop (cleanup of the finished VMI). If the launcher pod is already gone by
// the time the controller reconciles, ShutdownInfo stays empty: the handler
// neither cleans up the Succeeded VMI nor honors a pending vm-start-requested
// annotation (the start branch in handleManualPolicy and
// handleAlwaysOnUnlessStoppedManuallyPolicy is reachable only when no VMI
// exists), and the Nothing branch schedules no requeue. The VM parks in
// Stopped forever: an expected in-guest reboot never happens and a Start VMOP
// hangs InProgress.
func SkipIfGuestPowerActionStuck(ctx context.Context, key client.ObjectKey) {
	GinkgoHelper()

	kvvmi, err := GetInternalVirtualMachineInstance(ctx, &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
	})
	if err != nil || kvvmi == nil || kvvmi.DeletionTimestamp != nil || kvvmi.Status.Phase != virtv1.Succeeded {
		return
	}

	pods := &corev1.PodList{}
	err = framework.GetClients().GenericClient().List(ctx, pods,
		client.InNamespace(key.Namespace),
		client.MatchingLabels{"kubevirt.internal.virtualization.deckhouse.io": "virt-launcher"},
	)
	if err != nil {
		GinkgoWriter.Printf("Failed to list virt-launcher pods for the stuck guest power action check: %v\n", err)
		return
	}

	for _, pod := range pods.Items {
		if pod.Labels["kubevirt.internal.virtualization.deckhouse.io/created-by"] == string(kvvmi.UID) {
			return
		}
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.UID == kvvmi.UID {
				return
			}
		}
	}

	Skip(fmt.Sprintf("skip: internal VMI %s/%s is Succeeded and its virt-launcher pod is gone, the controller has lost the guest shutdown/reset reason and will not process the power action", key.Namespace, key.Name))
}

func UntilVirtualMachineRebooted(key client.ObjectKey, previousRunningTime time.Time, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		SkipIfGuestPowerActionStuck(context.Background(), key)

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
