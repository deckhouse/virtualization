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

package release

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	releaseTestPhaseEnv          = "RELEASE_TEST_PHASE"
	releaseTestPhasePreUpgrade   = "pre-upgrade"
	releaseTestPhasePostUpgrade  = "post-upgrade"
	releaseUpgradeContextPathEnv = "RELEASE_UPGRADE_CONTEXT_PATH"
	releaseNamespaceEnv          = "RELEASE_NAMESPACE"
	releaseUpgradeMigratesVMsEnv = "RELEASE_UPGRADE_MIGRATES_VMS"
)

var _ = Describe("CurrentReleaseSmoke", func() {
	It("should validate current release virtual machines", func() {
		switch getReleaseTestPhase() {
		case releaseTestPhasePostUpgrade:
			runPostUpgradeReleaseSmoke()
		default:
			runPreUpgradeReleaseSmoke()
		}
	})
})

type currentReleaseSmokeTest struct {
	framework      *framework.Framework
	vms            []*vmScenario
	dataDisks      []*dataDiskScenario
	attachments    []*attachmentScenario
	vmByName       map[string]*vmScenario
	dataDiskByName map[string]*dataDiskScenario
	iperfClient    *vmScenario
	iperfServer    *vmScenario
}

func runPreUpgradeReleaseSmoke() {
	f := framework.NewFramework("")
	namespace := ensureReleaseNamespace(f, releaseNamespaceName)

	test := newCurrentReleaseSmokeTest(f, namespace)
	test.createResources()
	test.verifyVMsReady()
	test.startLongRunningIPerf()
	test.writeUpgradeContext()
}

func runPostUpgradeReleaseSmoke() {
	namespace := mustGetEnv(releaseNamespaceEnv)
	f := framework.NewFramework("")
	test := newCurrentReleaseSmokeTest(f, namespace)

	test.verifyVMsSurvivedUpgrade()
	test.verifyIPerfContinuityAfterUpgrade()
	test.verifyEvictVMOPsCompleted()
}

func (t *currentReleaseSmokeTest) createResources() {
	ctx := context.Background()

	By("Creating root and hotplug virtual disks")
	Expect(t.framework.CreateWithDeferredDeletion(ctx, t.diskObjects()...)).To(Succeed())

	By("Creating virtual machines")
	Expect(t.framework.CreateWithDeferredDeletion(ctx, t.vmObjects()...)).To(Succeed())
	if runningVMs := t.initialRunningVMObjects(); len(runningVMs) > 0 {
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, runningVMs...)
	}
	if stoppedVMs := t.initialStoppedVMObjects(); len(stoppedVMs) > 0 {
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineStopped), framework.MiddleTimeout, stoppedVMs...)
	}

	By("Starting manual-policy virtual machines")
	for _, vmScenario := range t.manualStartVMs() {
		util.StartVirtualMachine(ctx, t.framework, vmScenario.vm)
	}
	if startedVMs := t.manualStartVMObjects(); len(startedVMs) > 0 {
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, startedVMs...)
	}

	By("Attaching hotplug disks")
	Expect(t.framework.CreateWithDeferredDeletion(ctx, t.attachmentObjects()...)).To(Succeed())
	util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MaxTimeout, t.attachmentObjects()...)

	By("Waiting for all disks to become ready after consumers appear")
	util.UntilObjectPhase(ctx, string(v1alpha2.DiskReady), framework.LongTimeout, t.diskObjects()...)
}

func (t *currentReleaseSmokeTest) verifyVMsReady() {
	By("Waiting for guest agent and SSH access")
	for _, vmScenario := range t.vms {
		t.expectGuestReady(vmScenario)
	}

	By("Checking attached disks inside guests")
	for _, vmScenario := range t.vms {
		By(fmt.Sprintf("Checking attached disks on %s", vmScenario.vm.Name))
		t.expectAdditionalDiskCount(vmScenario.vm, vmScenario.expectedAdditionalDisks)
	}
}

func (t *currentReleaseSmokeTest) verifyVMsSurvivedUpgrade() {
	By("Waiting for upgraded virtual machines to be running")
	util.UntilObjectPhase(context.Background(), string(v1alpha2.MachineRunning), framework.LongTimeout, t.vmObjects()...)

	By("Checking guest access after module upgrade")
	for _, vmScenario := range t.vms {
		t.expectGuestReady(vmScenario)
	}

	By("Checking attached disks after module upgrade")
	for _, vmScenario := range t.vms {
		t.expectAdditionalDiskCount(vmScenario.vm, vmScenario.expectedAdditionalDisks)
	}
}

func (t *currentReleaseSmokeTest) startLongRunningIPerf() {
	GinkgoHelper()

	waitForIPerfServerToStart(t.framework, t.iperfServer.vm)

	serverVM := t.getVirtualMachine(t.iperfServer.vm.Name, t.iperfServer.vm.Namespace)
	command := fmt.Sprintf(
		"nohup iperf3 -c %s -t 0 --json > %s 2>&1 </dev/null &",
		serverVM.Status.IPAddress,
		releaseIPerfReportPath,
	)
	_, err := t.framework.SSHCommand(
		t.iperfClient.vm.Name,
		t.iperfClient.vm.Namespace,
		command,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start long-running iperf3 client")

	waitForIPerfClientToStart(t.framework, t.iperfClient.vm)
}

func (t *currentReleaseSmokeTest) writeUpgradeContext() {
	GinkgoHelper()

	contextPath := os.Getenv(releaseUpgradeContextPathEnv)
	if contextPath == "" {
		return
	}

	err := os.MkdirAll(filepath.Dir(contextPath), 0o755)
	Expect(err).NotTo(HaveOccurred())

	contextData := releaseUpgradeContext{
		Namespace:       t.iperfClient.vm.Namespace,
		IPerfClientVM:   t.iperfClient.vm.Name,
		IPerfServerVM:   t.iperfServer.vm.Name,
		IPerfReportPath: releaseIPerfReportPath,
	}

	data, err := json.MarshalIndent(contextData, "", "  ")
	Expect(err).NotTo(HaveOccurred())

	err = os.WriteFile(contextPath, data, 0o644)
	Expect(err).NotTo(HaveOccurred())
}

func (t *currentReleaseSmokeTest) verifyIPerfContinuityAfterUpgrade() {
	GinkgoHelper()

	By("Checking that the iperf client is still running after upgrade")
	waitForIPerfClientToStart(t.framework, t.iperfClient.vm)

	By("Stopping the long-running iperf client after upgrade")
	stopIPerfClient(t.framework, t.iperfClient.vm)

	By("Validating the iperf report spans the module upgrade")
	report := getIPerfClientReport(t.framework, t.iperfClient.vm, releaseIPerfReportPath)
	Expect(isExpectedIPerfReportError(report.Error)).To(BeTrue(), "iperf3 report contains an unexpected error: %q", report.Error)

	if !upgradeMigratesVMs() {
		By("Skipping the migration window checks: the upgrade does not migrate virtual machines")
		Expect(report.End.SumSent.Bytes).To(BeNumerically(">", 0), "iperf3 client should send data")
		Expect(report.End.SumSent.BitsPerSecond).To(BeNumerically(">", 0), "iperf3 client should report throughput")

		return
	}

	By("Verifying the iperf test brackets the migration window (started before, stopped after)")
	migration := getMigrationWindow(t.framework, t.iperfServer.vm.Name, t.iperfServer.vm.Namespace)

	iperfStart, err := report.startTime()
	Expect(err).NotTo(HaveOccurred(), "iperf3 report start timestamp must be parseable")
	iperfEnd := report.endTime()

	Expect(iperfStart.Before(migration.start)).To(BeTrue(),
		"the iperf test must start before the migration starts (iperf start %s, migration start %s)", iperfStart, migration.start)
	Expect(iperfEnd.After(migration.end)).To(BeTrue(),
		"the iperf test must stop after the migration ends (iperf end %s, migration end %s)", iperfEnd, migration.end)

	upgradeStartedAt := migration.start.Unix()

	startedAt := int64(report.Start.Timestamp.Timesecs)
	endedAt := startedAt + int64(math.Ceil(report.End.SumSent.End))
	Expect(startedAt).To(BeNumerically("<=", upgradeStartedAt), "iperf3 should start before the module upgrade")
	Expect(endedAt).To(BeNumerically(">", upgradeStartedAt), "iperf3 should continue after the module upgrade")

	lowerIdx, upperIdx := continuityWindowBounds(startedAt, upgradeStartedAt, report.Intervals)
	Expect(upperIdx).To(BeNumerically(">=", lowerIdx), "iperf3 report must include intervals around the module upgrade")

	zeroIntervals := 0
	transmittedAroundUpgrade := int64(0)
	for idx := lowerIdx; idx <= upperIdx; idx++ {
		interval := report.Intervals[idx]
		if interval.Sum.Bytes == 0 {
			zeroIntervals++
			continue
		}
		transmittedAroundUpgrade += interval.Sum.Bytes
	}

	Expect(transmittedAroundUpgrade).To(BeNumerically(">", 0), "iperf3 should transmit data around the module upgrade")
	Expect(zeroIntervals).To(BeNumerically("<=", 1), "iperf3 should not be interrupted during the module upgrade")
	Expect(report.End.SumSent.Bytes).To(BeNumerically(">", 0), "iperf3 client should send data")
	Expect(report.End.SumSent.BitsPerSecond).To(BeNumerically(">", 0), "iperf3 client should report throughput")
}

func (t *currentReleaseSmokeTest) verifyEvictVMOPsCompleted() {
	GinkgoHelper()

	By("Verifying that all Evict VMOPs settled in the Completed phase after the upgrade")

	namespace := t.iperfClient.vm.Namespace
	vmops, err := t.framework.Clients.VirtClient().VirtualMachineOperations(namespace).List(context.Background(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	for _, vmop := range vmops.Items {
		if vmop.Spec.Type != v1alpha2.VMOPTypeEvict {
			continue
		}
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted),
			"Evict VMOP %s/%s must be in Completed phase, got %q", vmop.Spec.VirtualMachine, vmop.Name, vmop.Status.Phase)
	}
}

func (t *currentReleaseSmokeTest) getVirtualMachine(name, namespace string) *v1alpha2.VirtualMachine {
	GinkgoHelper()

	vm, err := t.framework.Clients.VirtClient().VirtualMachines(namespace).Get(context.Background(), name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	return vm
}

func getReleaseTestPhase() string {
	if phase := os.Getenv(releaseTestPhaseEnv); phase != "" {
		return phase
	}

	return releaseTestPhasePreUpgrade
}

// Upgrades between releases that ship the same virt-handler and virt-launcher
// never move a virtual machine. An unset value means the pipeline could not tell,
// so a migration is still expected.
func upgradeMigratesVMs() bool {
	return os.Getenv(releaseUpgradeMigratesVMsEnv) != "false"
}

func mustGetEnv(name string) string {
	value := os.Getenv(name)
	Expect(value).NotTo(BeEmpty(), "environment variable %s must be set", name)
	return value
}

func ensureReleaseNamespace(f *framework.Framework, namespace string) string {
	GinkgoHelper()

	nsClient := f.KubeClient().CoreV1().Namespaces()
	_, err := nsClient.Get(context.Background(), namespace, metav1.GetOptions{})
	switch {
	case err == nil:
		By(fmt.Sprintf("Namespace %q already exists, recreating it", namespace))
		err = nsClient.Delete(context.Background(), namespace, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			_, err := nsClient.Get(context.Background(), namespace, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("namespace %q is still deleting", namespace)
		}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())
	case !k8serrors.IsNotFound(err):
		Expect(err).NotTo(HaveOccurred())
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				framework.E2ELabel: "true",
			},
		},
	}
	_, err = nsClient.Create(context.Background(), ns, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
	By(fmt.Sprintf("Namespace %q has been created", namespace))

	return namespace
}
