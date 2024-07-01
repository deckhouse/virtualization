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
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

func ipamPath(file string) string {
	return path.Join(conf.Ipam.TestDataDir, file)
}

var _ = Describe("Ipam", func() {
	Context("VirtualMachineIPAddressClaim", Ordered, ContinueOnFailure, func() {
		AfterAll(func() {
			By("Removing resources for vmip tests")
			kubectl.Delete(conf.Ipam.TestDataDir, kc.DeleteOptions{})
		})
		GetLeaseNameFromClaim := func(manifestClaim string) string {
			res := kubectl.Get(manifestClaim, kc.GetOptions{Output: "jsonpath={.spec.virtualMachineIPAddressLeaseName}"})
			Expect(res.Error()).NotTo(HaveOccurred(), "failed get vmip from file %s.\n%s", manifestClaim, res.StdErr())
			return res.StdOut()
		}
		DeleteVMIP := func(manifest string) {
			res := kubectl.Delete(manifest, kc.DeleteOptions{})
			Expect(res.Error()).NotTo(HaveOccurred(), "failed delete vmip from file %s.\n%s", manifest, res.StdErr())

		}
		When("reclaimPolicy Delete", func() {
			filepath := ipamPath("vmip-delete.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{Phase: PhaseBound})
			It("Check lease exist", func() {
				leaseName := GetLeaseNameFromClaim(filepath)
				CheckField(kc.ResourceVMIPLease, leaseName, "jsonpath={'.status.phase'}", PhaseBound)
				DeleteVMIP(filepath)
				res := kubectl.GetResource(kc.ResourceVMIPLease, leaseName, kc.GetOptions{Namespace: conf.Namespace})
				Expect(res.Error()).To(HaveOccurred())
				Expect(res.StdErr()).To(ContainSubstring("not found"))
			})

		})
		When("reclaimPolicy Retain", func() {
			filepath := ipamPath("vmip-retain.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{Phase: PhaseBound})
			It("Check lease exist", func() {
				leaseName := GetLeaseNameFromClaim(filepath)
				CheckField(kc.ResourceVMIPLease, leaseName, "jsonpath={'.status.phase'}", PhaseBound)
				DeleteVMIP(filepath)
				CheckField(kc.ResourceVMIPLease, leaseName, "jsonpath={'.status.phase'}", PhaseReleased)
				kubectl.DeleteResource(kc.ResourceVMIPLease, leaseName, kc.DeleteOptions{Namespace: conf.Namespace})
			})
		})
	})
})
