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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineMigrationFail", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var f *framework.Framework

	BeforeEach(func() {
		// TODO: Re-enable the suite.
		Skip("skipped as flaky: fix the instability, then remove this skip")

		f = framework.NewFramework("vm-migration-fail")
		// The migration target pod is deliberately parked Unschedulable by a
		// nodeSelector pinning the VM to its current node, and the migration is
		// expected to fail with a timeout.
		f.TolerateUnschedulablePods()
		f.TolerateFailedMigrations()
		DeferCleanup(f.After)

		f.Before()
	})

	It("migration should fail via timeout because the target pod is unschedulable", func(ctx SpecContext) {
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
			vdbuilder.WithSize(ptr.To(resource.MustParse("400Mi"))),
		)
		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithBootloader(v1alpha2.BIOS),
			// Alpine does not fit the custom-image sizing of NewMinimalVM.
			vmbuilder.WithCPU(1, ptr.To("20%")),
			vmbuilder.WithMemory(*resource.NewQuantity(object.Mi512, resource.BinarySI)),
			vmbuilder.WithDisks(vdRoot),
			// The Alpine image needs cloud-init to start the guest agent.
			vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
		)

		Expect(f.CreateWithDeferredDeletion(ctx, vdRoot, vm)).To(Succeed())

		By("Wait until VM agent is ready", func() {
			util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		vm, err := f.VirtClient().VirtualMachines(vm.Namespace).Get(ctx, vm.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		nodeName := vm.Status.Node
		Expect(nodeName).NotTo(BeEmpty())

		vm.Spec.NodeSelector = map[string]string{
			hostnameLabelKey: nodeName,
		}

		vm, err = f.VirtClient().VirtualMachines(vm.Namespace).Update(ctx, vm, metav1.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("wait when nodeSelector will be synced to KVVMI", func() {
			Eventually(ctx, func(g Gomega) {
				kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, vm)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(kvvmi).NotTo(BeNil(), "internal VirtualMachineInstance is not found")
				g.Expect(kvvmi.Spec.NodeSelector[hostnameLabelKey]).To(Equal(nodeName), "nodeSelector is not synced yet")
			}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())
		})

		util.MigrateVirtualMachine(f, vm, vmopbuilder.WithAnnotation("kubevirt.internal.virtualization.deckhouse.io/migrationUnschedulablePodTimeoutSeconds", "10"))

		By("wait until virtualmachine migration will be failed with timeout", func() {
			Eventually(ctx, func(g Gomega) {
				vmops, err := f.VirtClient().VirtualMachineOperations(vm.Namespace).List(ctx, metav1.ListOptions{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(vmops.Items).To(HaveLen(1))
				vmop := vmops.Items[0]

				g.Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
				completedCond := meta.FindStatusCondition(vmop.Status.Conditions, string(vmopcondition.TypeCompleted))
				g.Expect(completedCond).NotTo(BeNil())
				g.Expect(completedCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(completedCond.Reason).To(Equal(vmopcondition.ReasonTargetUnschedulable.String()))
			}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())
		})
	})
})
