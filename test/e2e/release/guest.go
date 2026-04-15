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
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	lsblkJSONCommand    = "sudo lsblk --bytes --json --nodeps --output NAME,SIZE,TYPE,MOUNTPOINTS"
	rootDiskNameCommand = `root_source=$(findmnt -no SOURCE /); root_disk=$(lsblk -ndo PKNAME "$root_source" 2>/dev/null | head -n1); if [ -n "$root_disk" ]; then echo "$root_disk"; else lsblk -ndo NAME "$root_source" | head -n1; fi`
	maxCloudInitDiskSize = int64(4 * 1024 * 1024)
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
	util.UntilSSHReady(t.framework, vm, framework.LongTimeout)

	if vmScenario.skipGuestAgentCheck {
		By(fmt.Sprintf("Skipping strict guest agent check on %s", vm.Name))
		return
	}

	By(fmt.Sprintf("Waiting for guest agent on %s", vm.Name))
	util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
}

func (t *currentReleaseSmokeTest) expectAdditionalDiskCount(vm *v1alpha2.VirtualMachine, expectedCount int) {
	Eventually(func(g Gomega) {
		currentVM := &v1alpha2.VirtualMachine{}
		err := t.framework.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), currentVM)
		g.Expect(err).NotTo(HaveOccurred())

		attachedHotplugDisks := hotpluggedAttachedDiskCount(currentVM)
		g.Expect(attachedHotplugDisks).To(Equal(expectedCount))

		rootDiskName, err := t.rootDiskName(vm)
		g.Expect(err).NotTo(HaveOccurred())

		output, err := t.framework.SSHCommand(vm.Name, vm.Namespace, lsblkJSONCommand, framework.WithSSHTimeout(10*time.Second))
		g.Expect(err).NotTo(HaveOccurred())

		disks, err := parseLSBLKOutput(output)
		g.Expect(err).NotTo(HaveOccurred())

		actualCount := countAdditionalGuestDisks(disks, rootDiskName)
		g.Expect(actualCount).To(
			Equal(expectedCount),
			"VM %s/%s additional disk mismatch; root disk: %q; hotplugged block devices in status: %d; lsblk devices: %s",
			vm.Namespace,
			vm.Name,
			rootDiskName,
			attachedHotplugDisks,
			formatLSBLKDisks(disks),
		)
	}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())
}

func (t *currentReleaseSmokeTest) rootDiskName(vm *v1alpha2.VirtualMachine) (string, error) {
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

func hotpluggedAttachedDiskCount(vm *v1alpha2.VirtualMachine) int {
	count := 0
	for _, blockDevice := range vm.Status.BlockDeviceRefs {
		if !blockDevice.Hotplugged || !blockDevice.Attached || blockDevice.VirtualMachineBlockDeviceAttachmentName == "" {
			continue
		}
		count++
	}
	return count
}

func countAdditionalGuestDisks(disks []lsblkDevice, rootDiskName string) int {
	count := 0
	for _, disk := range disks {
		if disk.Type != "disk" {
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
