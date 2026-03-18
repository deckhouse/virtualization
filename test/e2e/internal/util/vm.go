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
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/controller"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

const (
	VmopE2ePrefix                  = "vmop-e2e"
	knownVolumeUpdateFailureReason = "VolumesUpdateError"
)

var knownKubeVirtClientSocketClosedRe = regexp.MustCompile(`(?is)virError\(Code=1,.*internal error:\s*client\s+socket\s+is\s+closed`)

var knownVDMigrationControllerRevertMessages = []string{
	"VirtualMachine is not running. Will be reverted.",
	"VirtualMachine is not migrating. Will be reverted.",
	"Target PersistentVolumeClaim is not found. Revert old PersistentVolumeClaim and remove migration condition.",
	"Target PersistentVolumeClaim is not bound. Revert old PersistentVolumeClaim and remove migration condition.",
}

type controllerLogMatch struct {
	PodName string
	Line    string
}

func IsKnownKubeVirtClientSocketClosedFailureReason(reason string) bool {
	return knownKubeVirtClientSocketClosedRe.MatchString(reason)
}

// TODO: remove temporary migration skip logic when issue "client socket is closed" is fixed:
func SkipIfKnownKubeVirtClientSocketClosedMigrationFailure(vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	if vm == nil {
		return
	}

	intvirtvmi, err := getInternalVirtualMachineInstance(vm)
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

func IsKnownVolumesUpdateFailureReason(reason string) bool {
	return reason == knownVolumeUpdateFailureReason
}

// TODO: remove temporary migration skip logic when known issue "VolumesUpdateError" is fixed:
func SkipIfKnownVolumesUpdateMigrationFailure(vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	if vm == nil {
		return
	}

	intvirtvmi, err := getInternalVirtualMachineInstance(vm)
	Expect(err).NotTo(HaveOccurred())
	if intvirtvmi == nil {
		return
	}

	// Prefer checking the concrete migratable condition, where volume update issues are expected.
	migratableCondition, exists := conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceIsMigratable, intvirtvmi.Status.Conditions)
	if exists && IsKnownVolumesUpdateFailureReason(migratableCondition.Reason) {
		Skip(fmt.Sprintf("skip due to known volume update migration issue for vm %s/%s: condition=%s, reason=%s, message=%s",
			vm.Namespace, vm.Name, migratableCondition.Type, migratableCondition.Reason, migratableCondition.Message))
	}
}

// TODO: remove temporary migration skip logic when both known issues are fixed:
// kubevirt "client socket is closed" and VolumesUpdateError.
func SkipIfKnownMigrationFailure(vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	SkipIfKnownKubeVirtClientSocketClosedMigrationFailure(vm)
	SkipIfKnownVolumesUpdateMigrationFailure(vm)
}

func WaitUntilConditionOrSkipKnownVDMigrationControllerRevert(timeout time.Duration, namespace string, condition func() error) {
	GinkgoHelper()

	waitStartedAt := time.Now()
	var lastErr error

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(context.Context) (bool, error) {
		lastErr = condition()
		return lastErr == nil, nil
	})
	if err == nil {
		return
	}

	if ctx.Err() == context.DeadlineExceeded {
		SkipIfKnownVDMigrationControllerRevertOnTimeout(namespace, waitStartedAt)
	}

	if lastErr != nil {
		Fail(fmt.Sprintf("timed out waiting for condition: %v", lastErr))
	}

	Expect(err).NotTo(HaveOccurred())
}

func SkipIfKnownVDMigrationControllerRevertOnTimeout(namespace string, since time.Time) {
	GinkgoHelper()

	match, err := findKnownVDMigrationControllerRevertLog(namespace, since)
	if err != nil {
		GinkgoWriter.Printf("Failed to inspect virtualization-controller logs for namespace %q: %v\n", namespace, err)
		return
	}
	if match == nil {
		return
	}

	Skip(fmt.Sprintf(
		"skip due to known virtualization-controller volume migration revert for namespace %s in pod %s: %s",
		namespace, match.PodName, match.Line,
	))
}

func findKnownVDMigrationControllerRevertLog(namespace string, since time.Time) (*controllerLogMatch, error) {
	ctx, cancel := context.WithTimeout(context.Background(), framework.ShortTimeout)
	defer cancel()

	pods, err := framework.GetClients().KubeClient().CoreV1().Pods(controller.VirtualizationNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", controller.VirtualizationController),
	})
	if err != nil {
		return nil, fmt.Errorf("list virtualization-controller pods: %w", err)
	}
	orderedPods, err := orderVirtualizationControllerPodsByLeader(ctx, pods.Items)
	if err != nil {
		GinkgoWriter.Printf("Failed to resolve virtualization-controller leader pod, fallback to all pods: %v\n", err)
		orderedPods = pods.Items
	}

	sinceTime := metav1.NewTime(since.Add(-5 * time.Second))
	for _, pod := range orderedPods {
		stream, err := framework.GetClients().KubeClient().CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: controller.VirtualizationController,
			SinceTime: &sinceTime,
		}).Stream(ctx)
		if err != nil {
			GinkgoWriter.Printf("Failed to read virtualization-controller logs from pod %s: %v\n", pod.Name, err)
			continue
		}

		logs, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read virtualization-controller logs from pod %s: %w", pod.Name, readErr)
		}
		if closeErr != nil {
			GinkgoWriter.Printf("Failed to close virtualization-controller log stream for pod %s: %v\n", pod.Name, closeErr)
		}

		if line := findKnownVDMigrationControllerRevertLine(string(logs), namespace); line != "" {
			return &controllerLogMatch{
				PodName: pod.Name,
				Line:    line,
			}, nil
		}
	}

	return nil, nil
}

func orderVirtualizationControllerPodsByLeader(ctx context.Context, pods []corev1.Pod) ([]corev1.Pod, error) {
	if len(pods) <= 1 {
		return pods, nil
	}
	if !isVirtualizationControllerLeaderElectionEnabled(pods) {
		return pods, nil
	}

	lease, err := framework.GetClients().KubeClient().CoordinationV1().Leases(controller.VirtualizationNamespace).Get(ctx, controller.LeaderElectionID, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return pods, nil
		}
		return nil, fmt.Errorf("get leader election lease %q: %w", controller.LeaderElectionID, err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" {
		return pods, nil
	}

	holderIdentity := *lease.Spec.HolderIdentity
	leaderIdx := slices.IndexFunc(pods, func(pod corev1.Pod) bool {
		return pod.Name == holderIdentity || strings.HasPrefix(holderIdentity, pod.Name+"_")
	})
	if leaderIdx == -1 {
		GinkgoWriter.Printf("Virtualization-controller leader lease holder %q does not match listed pods; fallback to all pods\n", holderIdentity)
		return pods, nil
	}

	orderedPods := make([]corev1.Pod, 0, len(pods))
	orderedPods = append(orderedPods, pods[leaderIdx])
	for i, pod := range pods {
		if i == leaderIdx {
			continue
		}
		orderedPods = append(orderedPods, pod)
	}

	return orderedPods, nil
}

func isVirtualizationControllerLeaderElectionEnabled(pods []corev1.Pod) bool {
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			if container.Name != controller.VirtualizationController {
				continue
			}
			return isLeaderElectionEnabledByArgs(container.Args)
		}
	}

	// The controller uses a default value of true when the flag is not passed.
	return true
}

func isLeaderElectionEnabledByArgs(args []string) bool {
	enabled := true

	for i, arg := range args {
		switch {
		case arg == "--leader-election" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--"):
			enabled = args[i+1] != "false"
		case arg == "--leader-election":
			enabled = true
		case arg == "--leader-election=true":
			enabled = true
		case arg == "--leader-election=false":
			enabled = false
		case strings.HasPrefix(arg, "--leader-election="):
			enabled = strings.TrimPrefix(arg, "--leader-election=") != "false"
		}
	}

	return enabled
}

func findKnownVDMigrationControllerRevertLine(logs, namespace string) string {
	for _, line := range strings.Split(logs, "\n") {
		if !strings.Contains(line, namespace) {
			continue
		}
		for _, message := range knownVDMigrationControllerRevertMessages {
			if strings.Contains(line, message) {
				return strings.TrimSpace(line)
			}
		}
	}
	return ""
}

func getInternalVirtualMachineInstance(vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	GinkgoHelper()

	obj := &rewrite.VirtualMachineInstance{}
	err := framework.GetClients().RewriteClient().Get(context.Background(), vm.Name, obj, rewrite.InNamespace(vm.Namespace))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return obj.VirtualMachineInstance, nil
}

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

func UntilVMMigrationSucceeded(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	WaitUntilConditionOrSkipKnownVDMigrationControllerRevert(timeout, key.Namespace, func() error {
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
	})
}

func MigrateVirtualMachine(f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...vmopbuilder.Option) {
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
}

func StartVirtualMachine(f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...vmopbuilder.Option) {
	GinkgoHelper()

	opts := []vmopbuilder.Option{
		vmopbuilder.WithGenerateName(fmt.Sprintf("%s-start-", VmopE2ePrefix)),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
		vmopbuilder.WithVirtualMachine(vm.Name),
	}
	opts = append(opts, options...)
	vmop := vmopbuilder.New(opts...)

	err := f.CreateWithDeferredDeletion(context.Background(), vmop)
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

	activePodName, err := getActivePodName(vm)
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

func getActivePodName(vm *v1alpha2.VirtualMachine) (string, error) {
	for _, pod := range vm.Status.VirtualMachinePods {
		if pod.Active {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf("no active pod found for virtual machine %s", vm.Name)
}

func UntilVirtualMachineRebooted(key client.ObjectKey, previousRunningTime time.Time, timeout time.Duration) {
	GinkgoHelper()

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
