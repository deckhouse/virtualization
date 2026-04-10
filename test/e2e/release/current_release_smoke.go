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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
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
)

var _ = Describe("CurrentReleaseSmoke", func() {
	It("should validate alpine virtual machines on current release", func() {
		f := framework.NewFramework("release-current")
		DeferCleanup(f.After)
		f.Before()

		test := newCurrentReleaseSmokeTest(f)

		By("Creating root and hotplug virtual disks")
		Expect(f.CreateWithDeferredDeletion(context.Background(), test.diskObjects()...)).To(Succeed())

		By("Creating virtual machines")
		Expect(f.CreateWithDeferredDeletion(context.Background(), test.vmObjects()...)).To(Succeed())
		util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, test.vmOneHotplug, test.vmTwoHotplug)
		util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.MiddleTimeout, test.vmAlwaysOff)

		By("Starting the manual-policy virtual machine")
		util.StartVirtualMachine(f, test.vmAlwaysOff)
		util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, test.vmAlwaysOff)

		By("Attaching hotplug disks")
		Expect(f.CreateWithDeferredDeletion(context.Background(), test.vmbdaOneHotplug, test.vmbdaReplicated, test.vmbdaLocalThin)).To(Succeed())
		util.UntilObjectPhase(
			string(v1alpha2.BlockDeviceAttachmentPhaseAttached),
			framework.LongTimeout,
			test.vmbdaOneHotplug,
			test.vmbdaReplicated,
			test.vmbdaLocalThin,
		)

		By("Waiting for all disks to become ready after consumers appear")
		util.UntilObjectPhase(string(v1alpha2.DiskReady), framework.LongTimeout, test.diskObjects()...)

		By("Waiting for guest agent and SSH access")
		test.expectGuestReady(test.vmAlwaysOff)
		test.expectGuestReady(test.vmOneHotplug)
		test.expectGuestReady(test.vmTwoHotplug)

		By("Checking attached disks inside guests")
		test.expectAdditionalDiskCount(test.vmAlwaysOff, 0)
		test.expectAdditionalDiskCount(test.vmOneHotplug, 1)
		test.expectAdditionalDiskCount(test.vmTwoHotplug, 2)
	})
})

type currentReleaseSmokeTest struct {
	framework *framework.Framework

	vmAlwaysOff  *v1alpha2.VirtualMachine
	vmOneHotplug *v1alpha2.VirtualMachine
	vmTwoHotplug *v1alpha2.VirtualMachine

	rootAlwaysOff  *v1alpha2.VirtualDisk
	rootOneHotplug *v1alpha2.VirtualDisk
	rootTwoHotplug *v1alpha2.VirtualDisk

	hotplugOne        *v1alpha2.VirtualDisk
	hotplugReplicated *v1alpha2.VirtualDisk
	hotplugLocalThin  *v1alpha2.VirtualDisk

	vmbdaOneHotplug *v1alpha2.VirtualMachineBlockDeviceAttachment
	vmbdaReplicated *v1alpha2.VirtualMachineBlockDeviceAttachment
	vmbdaLocalThin  *v1alpha2.VirtualMachineBlockDeviceAttachment
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

func newCurrentReleaseSmokeTest(f *framework.Framework) *currentReleaseSmokeTest {
	test := &currentReleaseSmokeTest{framework: f}
	namespace := f.Namespace().Name

	test.rootAlwaysOff = newRootDisk("vd-root-always-off", namespace)
	test.rootOneHotplug = newRootDisk("vd-root-one-hotplug", namespace)
	test.rootTwoHotplug = newRootDisk("vd-root-two-hotplug", namespace)

	test.hotplugOne = newHotplugDisk("vd-hotplug", namespace, replicatedStorageClass)
	test.hotplugReplicated = newHotplugDisk("vd-hotplug-replicated", namespace, replicatedStorageClass)
	test.hotplugLocalThin = newHotplugDisk("vd-hotplug-local-thin", namespace, localThinStorageClass)

	test.vmAlwaysOff = newVM("vm-always-off", namespace, v1alpha2.ManualPolicy, test.rootAlwaysOff.Name)
	test.vmOneHotplug = newVM("vm-one-hotplug", namespace, v1alpha2.AlwaysOnUnlessStoppedManually, test.rootOneHotplug.Name)
	test.vmTwoHotplug = newVM("vm-two-hotplug", namespace, v1alpha2.AlwaysOnPolicy, test.rootTwoHotplug.Name)

	test.vmbdaOneHotplug = object.NewVMBDAFromDisk("vmbda", test.vmOneHotplug.Name, test.hotplugOne)
	test.vmbdaReplicated = object.NewVMBDAFromDisk("vmbda1", test.vmTwoHotplug.Name, test.hotplugReplicated)
	test.vmbdaLocalThin = object.NewVMBDAFromDisk("vmbda2", test.vmTwoHotplug.Name, test.hotplugLocalThin)

	return test
}

func newRootDisk(name, namespace string) *v1alpha2.VirtualDisk {
	return object.NewVDFromCVI(
		name,
		namespace,
		object.PrecreatedCVIAlpineBIOS,
		vdbuilder.WithStorageClass(ptr.To(replicatedStorageClass)),
		vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
	)
}

func newHotplugDisk(name, namespace, storageClass string) *v1alpha2.VirtualDisk {
	return object.NewBlankVD(
		name,
		namespace,
		ptr.To(storageClass),
		ptr.To(resource.MustParse("100Mi")),
	)
}

func newVM(name, namespace string, runPolicy v1alpha2.RunPolicy, rootDiskName string) *v1alpha2.VirtualMachine {
	return vmbuilder.New(
		vmbuilder.WithName(name),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithCPU(1, ptr.To("20%")),
		vmbuilder.WithMemory(resource.MustParse("512Mi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
		vmbuilder.WithRunPolicy(runPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: rootDiskName,
		}),
	)
}

func (t *currentReleaseSmokeTest) diskObjects() []crclient.Object {
	return []crclient.Object{
		t.rootAlwaysOff,
		t.rootOneHotplug,
		t.rootTwoHotplug,
		t.hotplugOne,
		t.hotplugReplicated,
		t.hotplugLocalThin,
	}
}

func (t *currentReleaseSmokeTest) vmObjects() []crclient.Object {
	return []crclient.Object{
		t.vmAlwaysOff,
		t.vmOneHotplug,
		t.vmTwoHotplug,
	}
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
