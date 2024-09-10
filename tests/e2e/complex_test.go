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
	"strings"
	"time"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Complex test", Ordered, ContinueOnFailure, func() {
	Context("Virtualization resources", func() {
		When("Resources applied", func() {
			It("Result must have no error", func() {
				res := kubectl.Kustomize(conf.VirtualizationResources, kc.KustomizeOptions{})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
			})
		})
	})

	Context("Virtual images", func() {
		When("VI applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseReady), func() {
				CheckPhase("vi", PhaseReady)
			})
		})
	})

	Context("Disks", func() {
		When("VD applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseReady), func() {
				CheckPhase("vd", PhaseReady)
			})
		})
	})

	Context("Virtual machines IP addresses", func() {
		When("VMIP applied", func() {
			It("Patch custom IP address", func() {
				unassignedIP, err := FindUnassignedIP(mc.Spec.Settings.VirtualMachineCIDRs)
				Expect(err).NotTo(HaveOccurred())
				vmipMetadataName := fmt.Sprintf("%s-%s", namePrefix, "vm-custom-ip")
				mergePatch := fmt.Sprintf("{\"spec\":{\"staticIP\":\"%s\"}}", unassignedIP)
				MergePatchResource(kc.ResourceVMIP, vmipMetadataName, mergePatch)
			})
			It(fmt.Sprintf("Phase should be %s", PhaseBound), func() {
				CheckPhase("vmip", PhaseBound)
			})
		})
	})

	Context("Virtual machines", func() {
		When("VM applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseRunning), func() {
				CheckPhase("vm", PhaseRunning)
			})
		})
	})

	Context("Virtualmachine block device attachments", func() {
		When("VMBDA applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseAttached), func() {
				CheckPhase("vmbda", PhaseAttached)
			})
		})
	})

	Context("External connection", func() {
		When("VMs are running", func() {
			It("Wait 40 sec for sshd start in all running VMs", func() {
				time.Sleep(40 * time.Second)
			})
			It("All VMs must have to be connected to external network", func() {
				sshKeyPath := fmt.Sprintf("%s/id_ed", conf.Sshkeys)
				host := "https://flant.com"
				httpCode := "200"
				resourceType := kc.Resource("vm")
				output := "jsonpath='{.items[*].metadata.name}'"
				label := fmt.Sprintf("testcase=%s", namePrefix)

				res := kubectl.List(resourceType, kc.GetOptions{
					Namespace: conf.Namespace,
					Output:    output,
					Label:     label,
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vms := strings.Split(res.StdOut(), " ")
				CheckExternalConnection(sshKeyPath, host, httpCode, vms...)
			})
		})
	})
})
