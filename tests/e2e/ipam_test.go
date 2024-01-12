package e2e

import (
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"path"
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
		GetLeasNameFromClaim := func(manifestClaim string) string {
			res := kubectl.Get(manifestClaim, kc.GetOptions{Output: "jsonpath={.spec.leaseName}"})
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
			It("Check leas exist", func() {
				leasName := GetLeasNameFromClaim(filepath)
				CheckField(kc.ResourceVMIPLeas, leasName, "jsonpath={'.status.phase'}", PhaseBound)
				DeleteVMIP(filepath)
				res := kubectl.GetResource(kc.ResourceVMIPLeas, leasName, kc.GetOptions{Namespace: conf.Namespace})
				Expect(res.Error()).To(HaveOccurred())
				Expect(res.StdErr()).To(ContainSubstring("not found"))
			})

		})
		When("reclaimPolicy Retain", func() {
			filepath := ipamPath("vmip-retain.yaml")
			ItApplyWaitGet(filepath, ApplyWaitGetOptions{Phase: PhaseBound})
			It("Check leas exist", func() {
				leasName := GetLeasNameFromClaim(filepath)
				CheckField(kc.ResourceVMIPLeas, leasName, "jsonpath={'.status.phase'}", PhaseBound)
				DeleteVMIP(filepath)
				CheckField(kc.ResourceVMIPLeas, leasName, "jsonpath={'.status.phase'}", PhaseReleased)
				kubectl.DeleteResource(kc.ResourceVMIPLeas, leasName, kc.DeleteOptions{Namespace: conf.Namespace})
			})
		})
	})
})
