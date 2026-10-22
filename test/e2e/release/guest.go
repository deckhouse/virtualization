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
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	// lsblkJSONCommand is for cloud-init images (the iperf pair): the default
	// cloud user needs sudo. The custom image has no sudo (and no
	// cloud user), so lsblkJSONRootCommand runs the same query as root.
	lsblkJSONCommand     = "sudo lsblk --bytes --json --nodeps --output NAME,SIZE,TYPE,MOUNTPOINTS"
	lsblkJSONRootCommand = "lsblk --bytes --json --nodeps --output NAME,SIZE,TYPE,MOUNTPOINTS"
	rootDiskNameCommand  = `root_source=$(findmnt -no SOURCE /); root_disk=$(lsblk -ndo PKNAME "$root_source" 2>/dev/null | head -n1); if [ -n "$root_disk" ]; then echo "$root_disk"; else lsblk -ndo NAME "$root_source" | head -n1; fi`
	maxCloudInitDiskSize = int64(4 * 1024 * 1024)
	// customImageRootDiskName is the guest root disk of the custom
	// image: Deckhouse attaches the boot disk as the first virtio-scsi device
	// and the image mandates root=/dev/sda1, so the root disk is always sda.
	customImageRootDiskName = "sda"
)

type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name        string   `json:"name"`
	Size        int64    `json:"size"`
	Type        string   `json:"type"`
	Mountpoints []string `json:"mountpoints"`
}

func (t *currentReleaseSmokeTest) expectGuestReady(vmScenario *vmScenario) {
	vm := vmScenario.vm

	By(fmt.Sprintf("Waiting for SSH access on %s", vm.Name))
	if vmScenario.customImage {
		eventually.SSHReadyAsRoot(t.framework, vm, framework.LongTimeout)
	} else {
		eventually.SSHReady(t.framework, vm, framework.LongTimeout)
	}

	if vmScenario.skipGuestAgentCheck {
		By(fmt.Sprintf("Skipping strict guest agent check on %s", vm.Name))
		return
	}

	By(fmt.Sprintf("Waiting for guest agent on %s", vm.Name))
	if vmScenario.vmObs != nil {
		err := vmScenario.vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred(),
			"guest agent on %s should become ready", vm.Name)
		return
	}
	// In the post-upgrade phase the VM was created by the pre-upgrade run and
	// its agent is expected to already be ready; UntilVMAgentReady observes the
	// VM through a fresh watch, whose initial event carries the current state.
	util.UntilVMAgentReady(context.Background(), crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
}

// expectBlockDevices checks that every disk and image the VM was given (spec
// and hotplugged) is attached, both in the VM status and, for Linux guests,
// inside the guest: nothing may be lost across the module upgrade.
//
// EXCEPTION: the guest-side lsblk view converges asynchronously after the
// VMBDAs report Attached (there is no Kubernetes resource to observe for
// it), so a polling wait is used deliberately.
func (t *currentReleaseSmokeTest) expectBlockDevices(vmScenario *vmScenario) {
	vm := vmScenario.vm
	eventually.UntilAssertion(func(g Gomega) {
		currentVM := &v1alpha2.VirtualMachine{}
		err := t.framework.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), currentVM)
		g.Expect(err).NotTo(HaveOccurred())

		attached := attachedBlockDevices(currentVM)
		g.Expect(attached).To(ConsistOf(vmScenario.expectedBlockDevices),
			"VM %s/%s attached block devices mismatch", vm.Namespace, vm.Name)

		if vmScenario.skipDiskCheck {
			return
		}

		// The root disk is excluded from the guest count.
		expectedCount := len(vmScenario.expectedBlockDevices) - 1

		rootDiskName, err := t.rootDiskName(vmScenario)
		g.Expect(err).NotTo(HaveOccurred())

		lsblkCommand := lsblkJSONCommand
		sshOptions := []framework.SSHCommandOption{framework.WithSSHTimeout(10 * time.Second)}
		if vmScenario.customImage {
			lsblkCommand = lsblkJSONRootCommand
			sshOptions = append(sshOptions, framework.WithSSHUser("root"))
		}
		output, err := t.framework.SSHCommand(vm.Name, vm.Namespace, lsblkCommand, sshOptions...)
		g.Expect(err).NotTo(HaveOccurred())

		disks, err := parseLSBLKOutput(output)
		g.Expect(err).NotTo(HaveOccurred())

		actualCount := countAdditionalGuestBlockDevices(disks, rootDiskName)
		g.Expect(actualCount).To(
			Equal(expectedCount),
			"VM %s/%s additional block device mismatch; root disk: %q; attached block devices in status: %v; lsblk devices: %s",
			vm.Namespace,
			vm.Name,
			rootDiskName,
			attached,
			formatLSBLKDisks(disks),
		)
	}, framework.LongTimeout)
}

func (t *currentReleaseSmokeTest) rootDiskName(vmScenario *vmScenario) (string, error) {
	// The custom image always boots from /dev/sda (the image mandates
	// root=/dev/sda1), and it has no findmnt-driven cloud user path, so the
	// root disk name is known without asking the guest.
	if vmScenario.customImage {
		return customImageRootDiskName, nil
	}

	vm := vmScenario.vm
	output, err := t.framework.SSHCommand(vm.Name, vm.Namespace, rootDiskNameCommand, framework.WithSSHTimeout(10*time.Second))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

func parseLSBLKOutput(raw string) ([]lsblkDevice, error) {
	var output lsblkOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return nil, fmt.Errorf("parse lsblk json: %w", err)
	}

	return output.BlockDevices, nil
}

func attachedBlockDevices(vm *v1alpha2.VirtualMachine) []v1alpha2.BlockDeviceSpecRef {
	attached := make([]v1alpha2.BlockDeviceSpecRef, 0, len(vm.Status.BlockDeviceRefs))
	for _, blockDevice := range vm.Status.BlockDeviceRefs {
		if !blockDevice.Attached {
			continue
		}
		attached = append(attached, v1alpha2.BlockDeviceSpecRef{Kind: blockDevice.Kind, Name: blockDevice.Name})
	}
	return attached
}

// countAdditionalGuestBlockDevices counts disks and CD-ROMs (ISO images are
// attached as CD-ROMs) other than the root disk and the cloud-init disk.
func countAdditionalGuestBlockDevices(disks []lsblkDevice, rootDiskName string) int {
	count := 0
	for _, disk := range disks {
		if disk.Type != "disk" && disk.Type != "rom" {
			continue
		}
		if disk.Name == rootDiskName {
			continue
		}
		if disk.Size <= maxCloudInitDiskSize {
			continue
		}
		count++
	}
	return count
}

func formatLSBLKDisks(disks []lsblkDevice) string {
	if len(disks) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(disks))
	for _, disk := range disks {
		parts = append(parts, fmt.Sprintf("%s(type=%s,size=%d,mountpoints=%v)", disk.Name, disk.Type, disk.Size, disk.Mountpoints))
	}

	return "[" + strings.Join(parts, ", ") + "]"
}
