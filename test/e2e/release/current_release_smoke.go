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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	replicatedStorageClass = "nested-thin-r1"
	localThinStorageClass  = "nested-local-thin"
	lsblkJSONCommand       = "lsblk --bytes --json --nodeps --output NAME,SIZE,TYPE,MOUNTPOINTS"
	minDataDiskSizeBytes   = int64(50 * 1024 * 1024)
	defaultRootDiskSize    = "350Mi"
	defaultDataDiskSize    = "100Mi"
	iperfDurationSeconds   = 5
)

var _ = Describe("CurrentReleaseSmoke", func() {
	It("should validate current release virtual machines", func() {
		f := framework.NewFramework("release-current")
		f.Before()

		test := newCurrentReleaseSmokeTest(f)

		By("Creating root and hotplug virtual disks")
		Expect(f.CreateWithDeferredDeletion(context.Background(), test.diskObjects()...)).To(Succeed())

		By("Creating virtual machines")
		Expect(f.CreateWithDeferredDeletion(context.Background(), test.vmObjects()...)).To(Succeed())
		if runningVMs := test.initialRunningVMObjects(); len(runningVMs) > 0 {
			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, runningVMs...)
		}
		if stoppedVMs := test.initialStoppedVMObjects(); len(stoppedVMs) > 0 {
			util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.MiddleTimeout, stoppedVMs...)
		}

		By("Starting manual-policy virtual machines")
		for _, vmScenario := range test.manualStartVMs() {
			util.StartVirtualMachine(f, vmScenario.vm)
		}
		if startedVMs := test.manualStartVMObjects(); len(startedVMs) > 0 {
			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, startedVMs...)
		}

		By("Attaching hotplug disks")
		Expect(f.CreateWithDeferredDeletion(context.Background(), test.attachmentObjects()...)).To(Succeed())
		util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.LongTimeout, test.attachmentObjects()...)

		By("Waiting for all disks to become ready after consumers appear")
		util.UntilObjectPhase(string(v1alpha2.DiskReady), framework.LongTimeout, test.diskObjects()...)

		By("Waiting for guest agent and SSH access")
		for _, vmScenario := range test.vms {
			test.expectGuestReady(vmScenario.vm)
		}

		By("Checking attached disks inside guests")
		for _, vmScenario := range test.vms {
			test.expectAdditionalDiskCount(vmScenario.vm, vmScenario.expectedAdditionalDisks)
		}

		By("Running iperf smoke check between alpine guests")
		test.expectIPerfConnectivity()
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

type vmScenario struct {
	name                    string
	rootDiskName            string
	cviName                 string
	cloudInit               string
	runPolicy               v1alpha2.RunPolicy
	rootDiskSize            string
	expectedAdditionalDisks int

	rootDisk *v1alpha2.VirtualDisk
	vm       *v1alpha2.VirtualMachine
}

type dataDiskScenario struct {
	name         string
	storageClass string
	size         string

	disk *v1alpha2.VirtualDisk
}

type attachmentScenario struct {
	name     string
	vmName   string
	diskName string

	attachment *v1alpha2.VirtualMachineBlockDeviceAttachment
}

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name        string   `json:"name"`
	Size        int64    `json:"size"`
	Type        string   `json:"type"`
	Mountpoints []string `json:"mountpoints"`
}

type iperfReport struct {
	End iperfReportEnd `json:"end"`
}

type iperfReportEnd struct {
	SumSent     iperfReportSummary `json:"sum_sent"`
	SumReceived iperfReportSummary `json:"sum_received"`
}

type iperfReportSummary struct {
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
}

func newCurrentReleaseSmokeTest(f *framework.Framework) *currentReleaseSmokeTest {
	test := &currentReleaseSmokeTest{
		framework:      f,
		vmByName:       make(map[string]*vmScenario),
		dataDiskByName: make(map[string]*dataDiskScenario),
	}
	namespace := f.Namespace().Name

	test.vms = []*vmScenario{
		{
			name:                    "vm-alpine-manual",
			rootDiskName:            "vd-root-alpine-manual",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.AlpineCloudInit,
			runPolicy:               v1alpha2.ManualPolicy,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 0,
		},
		{
			name:                    "vm-alpine-single-hotplug",
			rootDiskName:            "vd-root-alpine-single-hotplug",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.AlpineCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 1,
		},
		{
			name:                    "vm-alpine-double-hotplug",
			rootDiskName:            "vd-root-alpine-double-hotplug",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.AlpineCloudInit,
			runPolicy:               v1alpha2.AlwaysOnPolicy,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 2,
		},
		{
			name:                    "vm-ubuntu-replicated-five",
			rootDiskName:            "vd-root-ubuntu-replicated-five",
			cviName:                 object.PrecreatedCVIUbuntu,
			cloudInit:               object.UbuntuCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			expectedAdditionalDisks: 5,
		},
		{
			name:                    "vm-ubuntu-mixed-five",
			rootDiskName:            "vd-root-ubuntu-mixed-five",
			cviName:                 object.PrecreatedCVIUbuntu,
			cloudInit:               object.UbuntuCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			expectedAdditionalDisks: 5,
		},
		{
			name:                    "vm-alpine-iperf-client",
			rootDiskName:            "vd-root-alpine-iperf-client",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.AlpineCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 2,
		},
		{
			name:                    "vm-alpine-iperf-server",
			rootDiskName:            "vd-root-alpine-iperf-server",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.PerfCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 0,
		},
	}

	test.dataDisks = []*dataDiskScenario{
		{name: "vd-data-alpine-single-hotplug-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-alpine-double-hotplug-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-alpine-double-hotplug-02-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-02-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-03-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-04-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-05-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-02-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-03-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-04-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-05-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-alpine-iperf-client-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-alpine-iperf-client-02-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
	}

	test.attachments = []*attachmentScenario{
		{name: "vmbda-alpine-single-hotplug-01", vmName: "vm-alpine-single-hotplug", diskName: "vd-data-alpine-single-hotplug-01-repl"},
		{name: "vmbda-alpine-double-hotplug-01", vmName: "vm-alpine-double-hotplug", diskName: "vd-data-alpine-double-hotplug-01-repl"},
		{name: "vmbda-alpine-double-hotplug-02", vmName: "vm-alpine-double-hotplug", diskName: "vd-data-alpine-double-hotplug-02-local"},
		{name: "vmbda-ubuntu-replicated-five-01", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-01-repl"},
		{name: "vmbda-ubuntu-replicated-five-02", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-02-repl"},
		{name: "vmbda-ubuntu-replicated-five-03", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-03-repl"},
		{name: "vmbda-ubuntu-replicated-five-04", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-04-repl"},
		{name: "vmbda-ubuntu-replicated-five-05", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-05-repl"},
		{name: "vmbda-ubuntu-mixed-five-01", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-01-repl"},
		{name: "vmbda-ubuntu-mixed-five-02", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-02-repl"},
		{name: "vmbda-ubuntu-mixed-five-03", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-03-local"},
		{name: "vmbda-ubuntu-mixed-five-04", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-04-local"},
		{name: "vmbda-ubuntu-mixed-five-05", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-05-local"},
		{name: "vmbda-alpine-iperf-client-01", vmName: "vm-alpine-iperf-client", diskName: "vd-data-alpine-iperf-client-01-repl"},
		{name: "vmbda-alpine-iperf-client-02", vmName: "vm-alpine-iperf-client", diskName: "vd-data-alpine-iperf-client-02-repl"},
	}

	for _, vmScenario := range test.vms {
		vmScenario.rootDisk = newRootDisk(vmScenario.rootDiskName, namespace, vmScenario.cviName, replicatedStorageClass, vmScenario.rootDiskSize)
		vmScenario.vm = newVM(vmScenario.name, namespace, vmScenario.runPolicy, vmScenario.rootDisk.Name, vmScenario.cloudInit)
		test.vmByName[vmScenario.name] = vmScenario
	}

	for _, diskScenario := range test.dataDisks {
		diskScenario.disk = newHotplugDisk(diskScenario.name, namespace, diskScenario.storageClass, diskScenario.size)
		test.dataDiskByName[diskScenario.name] = diskScenario
	}

	for _, attachmentScenario := range test.attachments {
		vmScenario := test.vmByName[attachmentScenario.vmName]
		diskScenario := test.dataDiskByName[attachmentScenario.diskName]
		attachmentScenario.attachment = object.NewVMBDAFromDisk(attachmentScenario.name, vmScenario.vm.Name, diskScenario.disk)
	}

	test.iperfClient = test.vmByName["vm-alpine-iperf-client"]
	test.iperfServer = test.vmByName["vm-alpine-iperf-server"]

	return test
}

func (s *vmScenario) expectedInitialPhase() string {
	if s.runPolicy == v1alpha2.ManualPolicy {
		return string(v1alpha2.MachineStopped)
	}

	return string(v1alpha2.MachineRunning)
}

func newRootDisk(name, namespace, cviName, storageClass, size string) *v1alpha2.VirtualDisk {
	options := []vdbuilder.Option{
		vdbuilder.WithStorageClass(ptr.To(storageClass)),
	}
	if size != "" {
		options = append(options, vdbuilder.WithSize(ptr.To(resource.MustParse(size))))
	}

	return object.NewVDFromCVI(name, namespace, cviName, options...)
}

func newHotplugDisk(name, namespace, storageClass, size string) *v1alpha2.VirtualDisk {
	return object.NewBlankVD(
		name,
		namespace,
		ptr.To(storageClass),
		ptr.To(resource.MustParse(size)),
	)
}

func newVM(name, namespace string, runPolicy v1alpha2.RunPolicy, rootDiskName, cloudInit string) *v1alpha2.VirtualMachine {
	return vmbuilder.New(
		vmbuilder.WithName(name),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithCPU(1, ptr.To("20%")),
		vmbuilder.WithMemory(resource.MustParse("512Mi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(cloudInit),
		vmbuilder.WithRunPolicy(runPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: rootDiskName,
		}),
	)
}

func (t *currentReleaseSmokeTest) diskObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms)+len(t.dataDisks))
	for _, vmScenario := range t.vms {
		objects = append(objects, vmScenario.rootDisk)
	}
	for _, diskScenario := range t.dataDisks {
		objects = append(objects, diskScenario.disk)
	}
	return objects
}

func (t *currentReleaseSmokeTest) vmObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms))
	for _, vmScenario := range t.vms {
		objects = append(objects, vmScenario.vm)
	}
	return objects
}

func (t *currentReleaseSmokeTest) attachmentObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.attachments))
	for _, attachmentScenario := range t.attachments {
		objects = append(objects, attachmentScenario.attachment)
	}
	return objects
}

func (t *currentReleaseSmokeTest) initialRunningVMObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms))
	for _, vmScenario := range t.vms {
		if vmScenario.expectedInitialPhase() == string(v1alpha2.MachineRunning) {
			objects = append(objects, vmScenario.vm)
		}
	}
	return objects
}

func (t *currentReleaseSmokeTest) initialStoppedVMObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms))
	for _, vmScenario := range t.vms {
		if vmScenario.expectedInitialPhase() == string(v1alpha2.MachineStopped) {
			objects = append(objects, vmScenario.vm)
		}
	}
	return objects
}

func (t *currentReleaseSmokeTest) manualStartVMs() []*vmScenario {
	manualVMs := make([]*vmScenario, 0)
	for _, vmScenario := range t.vms {
		if vmScenario.runPolicy == v1alpha2.ManualPolicy {
			manualVMs = append(manualVMs, vmScenario)
		}
	}
	return manualVMs
}

func (t *currentReleaseSmokeTest) manualStartVMObjects() []crclient.Object {
	objects := make([]crclient.Object, 0)
	for _, vmScenario := range t.manualStartVMs() {
		objects = append(objects, vmScenario.vm)
	}
	return objects
}

func (t *currentReleaseSmokeTest) expectGuestReady(vm *v1alpha2.VirtualMachine) {
	By(fmt.Sprintf("Waiting for guest agent on %s", vm.Name))
	util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

	By(fmt.Sprintf("Waiting for SSH access on %s", vm.Name))
	util.UntilSSHReady(t.framework, vm, framework.LongTimeout)
}

func (t *currentReleaseSmokeTest) expectAdditionalDiskCount(vm *v1alpha2.VirtualMachine, expectedCount int) {
	Eventually(func(g Gomega) {
		output, err := t.framework.SSHCommand(vm.Name, vm.Namespace, lsblkJSONCommand, framework.WithSSHTimeout(10*time.Second))
		g.Expect(err).NotTo(HaveOccurred())

		disks, err := parseLSBLKOutput(output)
		g.Expect(err).NotTo(HaveOccurred())

		count := 0
		for _, disk := range disks {
			if disk.Type != "disk" {
				continue
			}
			if disk.Size <= minDataDiskSizeBytes {
				continue
			}
			if hasMountpoint(disk.Mountpoints, "/") {
				continue
			}
			count++
		}

		g.Expect(count).To(Equal(expectedCount))
	}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())
}

func parseLSBLKOutput(raw string) ([]lsblkDevice, error) {
	var output lsblkOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return nil, fmt.Errorf("parse lsblk json: %w", err)
	}

	return output.BlockDevices, nil
}

func hasMountpoint(mountpoints []string, expected string) bool {
	for _, mountpoint := range mountpoints {
		if mountpoint == expected {
			return true
		}
	}

	return false
}

func (t *currentReleaseSmokeTest) expectIPerfConnectivity() {
	GinkgoHelper()

	waitForIPerfServerToStart(t.framework, t.iperfServer.vm)

	serverVM := t.getVirtualMachine(t.iperfServer.vm.Name, t.iperfServer.vm.Namespace)
	command := fmt.Sprintf("iperf3 -c %s -t %d --json", serverVM.Status.IPAddress, iperfDurationSeconds)
	output, err := t.framework.SSHCommand(
		t.iperfClient.vm.Name,
		t.iperfClient.vm.Namespace,
		command,
		framework.WithSSHTimeout((iperfDurationSeconds+10)*time.Second),
	)
	Expect(err).NotTo(HaveOccurred(), "failed to run iperf3 client")

	report, err := parseIPerfReport(output)
	Expect(err).NotTo(HaveOccurred(), "failed to parse iperf3 client output")
	Expect(report.End.SumSent.Bytes).To(BeNumerically(">", 0), "iperf3 client should send data")
	Expect(report.End.SumSent.BitsPerSecond).To(BeNumerically(">", 0), "iperf3 client should report throughput")
}

func (t *currentReleaseSmokeTest) getVirtualMachine(name, namespace string) *v1alpha2.VirtualMachine {
	GinkgoHelper()

	vm, err := t.framework.Clients.VirtClient().VirtualMachines(namespace).Get(context.Background(), name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	return vm
}

func waitForIPerfServerToStart(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	command := "rc-service iperf3 status --nocolor"
	Eventually(func() error {
		stdout, err := f.SSHCommand(vm.Name, vm.Namespace, command)
		if err != nil {
			return fmt.Errorf("cmd: %s\nstderr: %w", command, err)
		}
		if strings.Contains(stdout, "status: started") {
			return nil
		}
		return fmt.Errorf("iperf3 server is not started yet: %s", stdout)
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
}

func parseIPerfReport(raw string) (*iperfReport, error) {
	var report iperfReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return nil, fmt.Errorf("parse iperf3 json: %w", err)
	}

	return &report, nil
}
