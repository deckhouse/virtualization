/*
Copyright 2024 Flant JSC

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

package e2e

import (
	"fmt"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var (
	hotplugLabel          = map[string]string{"vm": "hotplug"}
	automaticHotplugLabel = map[string]string{"vm": "automatic-with-hotplug"}
)

// TODO: Remove this flow when migration problem for virtual machines with hotplug disk will be fixed.
func GetHotplugVirtualMachines() ([]string, error) {
	vms := make([]string, 0)
	hotplugRes := kubectl.List(kc.ResourceVM, kc.GetOptions{
		Labels:    hotplugLabel,
		Namespace: conf.Namespace,
		Output:    "jsonpath='{.items[*].metadata.name}'",
	})
	if hotplugRes.Error() != nil {
		return nil, fmt.Errorf(hotplugRes.StdErr())
	}
	vms = append(vms, strings.Split(hotplugRes.StdOut(), " ")...)

	automaticHotplugRes := kubectl.List(kc.ResourceVM, kc.GetOptions{
		Labels:    automaticHotplugLabel,
		Namespace: conf.Namespace,
		Output:    "jsonpath='{.items[*].metadata.name}'",
	})
	if automaticHotplugRes.Error() != nil {
		return nil, fmt.Errorf(automaticHotplugRes.StdErr())
	}
	vms = append(vms, strings.Split(automaticHotplugRes.StdOut(), " ")...)

	return vms, nil
}

func FilterVms(vms, excludedVms []string) []string {
	filteredVms := make([]string, 0, len(vms)-len(excludedVms))
	for _, vm := range vms {
		if !slices.Contains(excludedVms, vm) {
			filteredVms = append(filteredVms, vm)
		}
	}
	return filteredVms
}

var _ = Describe("Complex test", Ordered, ContinueOnFailure, func() {
	Context("When virtualization resources are applied:", func() {
		It("must has no error", func() {
			res := kubectl.Kustomize(conf.TestData.VirtualizationResources, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied:", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be are in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVI, PhaseReady, kc.GetOptions{
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When cluster virtual images are applied:", func() {
		It("checks CVIs phases", func() {
			By(fmt.Sprintf("CVIs should be are in %s phases", PhaseReady))
			WaitPhase(kc.ResourceCVI, PhaseReady, kc.GetOptions{
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be are in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines IP addresses are applied:", func() {
		It("patches custom VMIP with unassigned address", func() {
			unassignedIP, err := FindUnassignedIP(mc.Spec.Settings.VirtualMachineCIDRs)
			Expect(err).NotTo(HaveOccurred())
			vmipMetadataName := fmt.Sprintf("%s-%s", namePrefix, "vm-custom-ip")
			mergePatch := fmt.Sprintf("{\"spec\":{\"staticIP\":\"%s\"}}", unassignedIP)
			MergePatchResource(kc.ResourceVMIP, vmipMetadataName, mergePatch)
		})
		It("checks VMIPs phases", func() {
			By(fmt.Sprintf("VMIPs should be are in %s phases", PhaseBound))
			WaitPhase(kc.ResourceVMIP, PhaseBound, kc.GetOptions{
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be are in %s phases", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machine block device attachments are applied:", func() {
		It("checks VMBDAs phases", func() {
			By(fmt.Sprintf("VMBDAs should be are in %s phases", PhaseAttached))
			WaitPhase(kc.ResourceVMBDA, PhaseAttached, kc.GetOptions{
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Describe("External connection", func() {
		Context(fmt.Sprintf("When VMs are in %s phases", PhaseRunning), func() {
			It("checks VMs external connectivity", func() {
				sshKeyPath := fmt.Sprintf("%s/id_ed", conf.TestData.Sshkeys)

				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckExternalConnection(sshKeyPath, externalHost, httpStatusOk, vms...)
			})
		})
	})

	Describe("Migrations", func() {
		Context(fmt.Sprintf("When VMs are in %s phases:", PhaseRunning), func() {
			It("starts migrations", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				hotplugVms, err := GetHotplugVirtualMachines()
				Expect(err).NotTo(HaveOccurred(), err)
				filteredVms := FilterVms(vms, hotplugVms)
				MigrateVirtualMachines(filteredVms...)
			})
		})

		Context("When VMs migrations are applied:", func() {
			It("checks VMs and INTVIRTVMIMs phases", func() {
				By(fmt.Sprintf("INTVIRTVMIMs should be are in %s phases", PhaseSucceeded))
				WaitPhase(kc.ResourceKubevirtVMIM, PhaseSucceeded, kc.GetOptions{
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				By(fmt.Sprintf("VMs should be are in %s phase", PhaseRunning))
				WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
			})

			It("checks VMs external connection after migrations", func() {
				sshKeyPath := fmt.Sprintf("%s/id_ed", conf.TestData.Sshkeys)

				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckExternalConnection(sshKeyPath, externalHost, httpStatusOk, vms...)
			})
		})
	})
})
