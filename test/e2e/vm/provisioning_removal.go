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

package vm

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// cloudInitVolumeName is the name of the cloud-init volume and disk in the
// underlying KubeVirt VMI (see kvbuilder.CloudInitDiskName).
const cloudInitVolumeName = "cloudinit"

var _ = Describe("VirtualMachineProvisioningRemoval", Label(precheck.NoPrecheck), func() {
	It("should remove cloud-init provisioning from a running VM without a restart", func() {
		ctx := context.Background()
		f := framework.NewFramework("vm-provisioning-removal")
		DeferCleanup(f.After)
		f.Before()

		t := newProvisioningRemovalTest(f)

		By("Environment preparation")
		t.GenerateResources()
		err := f.CreateWithDeferredDeletion(ctx, t.VM, t.VDRoot)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until VM agent is ready")
		util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

		By("Checking the running VMI carries the cloud-init disk")
		kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvmi).NotTo(BeNil())
		Expect(vmiHasVolume(kvvmi, cloudInitVolumeName)).To(BeTrue(), "cloud-init volume should be present before removal")
		Expect(vmiHasDisk(kvvmi, cloudInitVolumeName)).To(BeTrue(), "cloud-init disk should be present before removal")
		vmiUID := kvvmi.UID

		initialNode, err := util.GetVMNode(ctx, f, t.VM)
		Expect(err).NotTo(HaveOccurred())

		By("Removing provisioning from the VM spec")
		patchset := patch.NewJSONPatch(patch.WithRemove("/spec/provisioning"))
		patchBytes, err := patchset.Bytes()
		Expect(err).NotTo(HaveOccurred())
		t.VM, err = f.VirtClient().VirtualMachines(t.VM.Namespace).Patch(ctx, t.VM.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until the cloud-init disk is hot-detached from the running VMI")
		Eventually(func(g Gomega) {
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(vmiHasVolume(kvvmi, cloudInitVolumeName)).To(BeFalse(), "cloud-init volume should be detached")
			g.Expect(vmiHasDisk(kvvmi, cloudInitVolumeName)).To(BeFalse(), "cloud-init disk should be detached")
			g.Expect(kvvmi.UID).To(Equal(vmiUID), "VMI must not be recreated (no restart)")
		}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())

		By("Checking that no restart is required and the VM was not restarted")
		Consistently(func(g Gomega) {
			err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			_, exists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, t.VM.Status.Conditions)
			g.Expect(exists).To(BeFalse(), "removing cloud-init must not require a restart")
			g.Expect(t.VM.Status.RestartAwaitingChanges).To(BeNil())

			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(kvvmi.UID).To(Equal(vmiUID), "VMI must not be recreated (no restart)")
		}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())

		util.ExpectNoVMOperationsForVirtualMachine(ctx, f, t.VM)
		util.ExpectVMOnNode(ctx, f, t.VM, initialNode)

		By("Checking the VM is still reachable over SSH")
		util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
	})
})

type provisioningRemovalTest struct {
	Framework *framework.Framework

	VM     *v1alpha2.VirtualMachine
	VDRoot *v1alpha2.VirtualDisk
}

func newProvisioningRemovalTest(f *framework.Framework) *provisioningRemovalTest {
	return &provisioningRemovalTest{Framework: f}
}

func (t *provisioningRemovalTest) GenerateResources() {
	t.VDRoot = object.NewVDFromCVI("vd-root", t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS)

	// NewMinimalVM sets cloud-init provisioning (UserData) by default. Manual
	// restart approval makes a wrongly-required restart observable as the
	// AwaitingRestartToApplyConfiguration condition instead of an auto-restart.
	t.VM = object.NewMinimalVM("vm", t.Framework.Namespace().Name,
		vmbuilder.WithDisks(t.VDRoot),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
	)
}

func vmiHasVolume(vmi *virtv1.VirtualMachineInstance, name string) bool {
	for _, v := range vmi.Spec.Volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}

func vmiHasDisk(vmi *virtv1.VirtualMachineInstance, name string) bool {
	for _, d := range vmi.Spec.Domain.Devices.Disks {
		if d.Name == name {
			return true
		}
	}
	return false
}
