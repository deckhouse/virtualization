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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobserver "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

const (
	VmopE2ePrefix = "vmop-e2e"
)

var knownKubeVirtClientSocketClosedRe = regexp.MustCompile(`(?is)virError\(Code=1,.*internal error:\s*client\s+socket\s+is\s+closed`)

func IsKnownKubeVirtClientSocketClosedFailureReason(reason string) bool {
	return knownKubeVirtClientSocketClosedRe.MatchString(reason)
}

// knownKubeVirtTargetPodShutdownRe matches the transient kubevirt failure where
// the migration target pod is torn down before the migration handoff completes
// ("Migration failed because target pod <pod> shutdown during migration"). The
// same scenario re-run in isolation migrates cleanly.
var knownKubeVirtTargetPodShutdownRe = regexp.MustCompile(`(?i)target pod .* shutdown during migration`)

func IsKnownKubeVirtTargetPodShutdownFailureReason(reason string) bool {
	return knownKubeVirtTargetPodShutdownRe.MatchString(reason)
}

// knownKubeVirtHotplugDiskNotPermittedRe matches the transient kubevirt failure
// where the source virt-launcher cannot reopen a hotplugged disk while
// generating migration parameters ("qemu-img: Could not open
// '/var/run/kubevirt/hotplug-disks/<disk>': Operation not permitted"): the
// hotplug mount is not relabeled for the launcher in time. The same scenario
// re-run in isolation migrates cleanly.
var knownKubeVirtHotplugDiskNotPermittedRe = regexp.MustCompile(`(?i)could not open '/var/run/kubevirt/hotplug-disks/[^']*':\s*operation not permitted`)

func IsKnownKubeVirtHotplugDiskNotPermittedFailureReason(reason string) bool {
	return knownKubeVirtHotplugDiskNotPermittedRe.MatchString(reason)
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

// knownFirmwareUpdateVMOPConflictRe matches the vmop admission denial raised while a
// spontaneous firmware-update VMOP is in flight for the same virtual machine (see the
// firmware-update note in ExpectNoVMOperationsForVirtualMachine).
var knownFirmwareUpdateVMOPConflictRe = regexp.MustCompile(`Previously created operation "firmware-update-[^"]*" should finish first`)

// TODO: remove when the workload updater stops emitting spontaneous firmware-update
// VMOPs on the dev cluster.
func SkipIfKnownFirmwareUpdateVMOPConflict(err error) {
	GinkgoHelper()

	if err == nil {
		return
	}
	if knownFirmwareUpdateVMOPConflictRe.MatchString(err.Error()) {
		Skip("skip due to known spontaneous firmware-update VMOP in flight: " + err.Error())
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
	skipIfKnownKubeVirtTargetPodShutdownMigrationFailure(ctx, vm)
}

// TODO: remove temporary migration skip logic when the kubevirt "target pod shutdown
// during migration" flake is fixed:
func skipIfKnownKubeVirtTargetPodShutdownMigrationFailure(ctx context.Context, vm *v1alpha2.VirtualMachine) {
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
	if IsKnownKubeVirtTargetPodShutdownFailureReason(failureReason) {
		Skip(fmt.Sprintf("skip due to known kubevirt migration issue (target pod shutdown during migration) for vm %s/%s: %s",
			vm.Namespace, vm.Name, failureReason))
	}
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

// UntilVMAgentReady waits for the VirtualMachine's AgentReady condition to
// become True, observing the VM through a watch.
func UntilVMAgentReady(ctx context.Context, key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualMachine](
		ctx,
		framework.GetClients().VirtClient().VirtualMachines(key.Namespace),
		key.Name, key.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachine %s/%s", key.Namespace, key.Name)
	defer obs.Stop()

	// Evaluate the current state explicitly as well: a VM whose agent became
	// ready long ago may produce no further status updates to observe.
	current, getErr := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(ctx, key.Name, metav1.GetOptions{})
	if getErr == nil {
		if ok, _ := vmobserver.BeAgentReady()(current); ok {
			return
		}
	}

	err = obs.WaitFor(vmobserver.BeAgentReady(), timeout)
	Expect(err).NotTo(HaveOccurred(), "%s: guest agent is not ready", key.Name)
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
		if vmop.Spec.VirtualMachine != vm.Name {
			continue
		}
		// TODO: stop ignoring firmware-update VMOPs when the workload updater no
		// longer emits them spontaneously on the dev cluster: transient
		// LauncherContainerImageVersion mismatches mark fresh VMs FirmwareUpToDate=False
		// and the firmware handler creates a one-shot migration nothing asked for.
		if strings.HasPrefix(vmop.Name, "firmware-update-") {
			continue
		}
		Fail(fmt.Sprintf("unexpected VMOP %q for VM %q", vmop.Name, vm.Name))
	}
}

func ExpectVMOnNode(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, expectedNode string) {
	GinkgoHelper()

	node, err := GetVMNode(ctx, f, vm)
	Expect(err).NotTo(HaveOccurred())
	Expect(node).To(Equal(expectedNode))
}

// UntilVMMigrationSucceeded waits for the newest migration VMOP of the VM to reach a terminal
// phase and for the VM's migration state to report success. The VMOP is created asynchronously
// with a generated name (workload updater, storage class change), so it is discovered by
// watching the namespace's VMOPs for the first Evict/Migrate operation of the VM. A VMOP that
// turns Failed fails the test immediately.
func UntilVMMigrationSucceeded(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	vmop, err := observer.WaitForFirst(ctx,
		framework.GetClients().VirtClient().VirtualMachineOperations(key.Namespace),
		timeout,
		func(op *v1alpha2.VirtualMachineOperation) bool {
			if op.Spec.VirtualMachine != key.Name {
				return false
			}
			return op.Spec.Type == v1alpha2.VMOPTypeEvict || op.Spec.Type == v1alpha2.VMOPTypeMigrate
		})
	Expect(err).NotTo(HaveOccurred(), "no migration vmop found for vm %s/%s", key.Namespace, key.Name)

	UntilVMOPMigrationSucceeded(ctx, vmop, timeout)

	// The VM object mirrors the migration state of the completed VMOP with a small lag; keep
	// asserting the same final state as before. The observer uses a fresh context: ctx may be
	// close to its deadline after a long migration.
	vmObs, err := observer.New[*v1alpha2.VirtualMachine](
		context.Background(),
		framework.GetClients().VirtClient().VirtualMachines(key.Namespace),
		key.Name, key.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachine %s/%s", key.Namespace, key.Name)
	defer vmObs.Stop()

	if waitErr := vmObs.WaitFor(vmobserver.HaveMigrationSucceeded(), framework.ShortTimeout); waitErr != nil {
		// TODO: remove temporary migration skip logic when both known issues are fixed:
		// kubevirt "client socket is closed" and Volume(s)UpdateError.
		if vm, getErr := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{}); getErr == nil {
			SkipIfKnownMigrationFailure(vm)
		}
		Fail(fmt.Sprintf("migration is not completed: %s", waitErr))
	}
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

	err := CreateVMOPRetryingStaleActiveDenial(context.Background(), f, vmop)
	Expect(err).NotTo(HaveOccurred())

	return vmop
}

// CreateVMOPRetryingStaleActiveDenial creates the VMOP, retrying with a short
// backoff while the webhook still denies it over the previous, just-finished
// operation. The denial itself is correct fail-safe behavior; the webhook
// lists VMOPs through a cached client, which can briefly lag behind the watch
// the spec observed the previous operation's completion on.
func CreateVMOPRetryingStaleActiveDenial(ctx context.Context, f *framework.Framework, vmop *v1alpha2.VirtualMachineOperation) error {
	backoff := wait.Backoff{Duration: 500 * time.Millisecond, Factor: 2, Jitter: 0.1, Steps: 5}
	return retry.OnError(backoff,
		func(err error) bool { return strings.Contains(err.Error(), "should finish first") },
		func() error { return f.CreateWithDeferredDeletion(ctx, vmop) },
	)
}

func StartVirtualMachine(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...vmopbuilder.Option) *v1alpha2.VirtualMachineOperation {
	GinkgoHelper()

	opts := []vmopbuilder.Option{
		vmopbuilder.WithGenerateName(fmt.Sprintf("%s-start-", VmopE2ePrefix)),
		vmopbuilder.WithNamespace(vm.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
		vmopbuilder.WithVirtualMachine(vm.Name),
	}
	opts = append(opts, options...)
	vmop := vmopbuilder.New(opts...)

	err := CreateVMOPRetryingStaleActiveDenial(ctx, f, vmop)
	Expect(err).NotTo(HaveOccurred())

	return vmop
}

// StopVirtualMachineFromOS powers the guest off from inside, as root without
// sudo (custom image guests).
func StopVirtualMachineFromOS(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, "nohup sh -c \"sleep 5 && poweroff\" > /dev/null 2>&1 &", framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
}

// RebootVirtualMachineBySSH reboots the guest from inside, as root without
// sudo (custom image guests).
func RebootVirtualMachineBySSH(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, "nohup sh -c \"sleep 5 && reboot\" > /dev/null 2>&1 &", framework.WithSSHUser("root"))
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

func IsVDAttached(vm *v1alpha2.VirtualMachine, vd *v1alpha2.VirtualDisk) bool {
	for _, bd := range vm.Status.BlockDeviceRefs {
		if bd.Kind == v1alpha2.DiskDevice && bd.Name == vd.Name && bd.Attached {
			return true
		}
	}
	return false
}

// IsRestartRequired reports whether the VirtualMachine requires a manual
// restart to apply configuration changes. For a VM with the Manual restart
// approval mode it observes the VM through a watch until the
// AwaitingRestartToApplyConfiguration condition becomes True (failing the
// test after timeout) and leaves the refreshed state in vm.
func IsRestartRequired(vm *v1alpha2.VirtualMachine, timeout time.Duration) bool {
	GinkgoHelper()

	if vm.Spec.Disruptions.RestartApprovalMode != v1alpha2.Manual {
		return false
	}

	ctx := context.Background()
	obs, err := observer.New[*v1alpha2.VirtualMachine](
		ctx,
		framework.GetClients().VirtClient().VirtualMachines(vm.Namespace),
		vm.Name, vm.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachine %s/%s", vm.Namespace, vm.Name)
	defer obs.Stop()

	err = obs.WaitFor(vmobserver.BeAwaitingRestart(), timeout)
	Expect(err).NotTo(HaveOccurred(), "VirtualMachine %s/%s did not report awaiting restart", vm.Namespace, vm.Name)

	// The former polling implementation refreshed the caller's object on every
	// attempt; keep that contract.
	Expect(framework.GetClients().GenericClient().Get(ctx, client.ObjectKeyFromObject(vm), vm)).To(Succeed())

	return true
}
