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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	restartConsistencyDuration = 5 * time.Second
	restartConsistencyPolling  = time.Second
	hotplugPolling             = 5 * time.Second
)

var _ = Describe("VirtualMachineBlockDeviceHotplugAttach", Label(precheck.NoPrecheck), func() {
	f := framework.NewFramework("vm-bd-hotplug-attach")

	var (
		vm               *v1alpha2.VirtualMachine
		vdBlank          *v1alpha2.VirtualDisk
		initialDiskCount int
	)

	BeforeEach(func() {
		DeferCleanup(f.After)
		f.Before()
		vm, _, vdBlank, initialDiskCount = setupVM(f, false)
	})

	It("should hotplug a disk without restart", func() {
		By("Adding blank disk to spec.blockDeviceRefs")
		err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
		Expect(err).NotTo(HaveOccurred())

		vm.Spec.BlockDeviceRefs = append(vm.Spec.BlockDeviceRefs, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: vdBlank.Name,
		})
		err = f.Clients.GenericClient().Update(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying no restart is required")
		Consistently(func(g Gomega) {
			g.Expect(f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)).To(Succeed())
			needRestart, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
			g.Expect(needRestart.Status).NotTo(Equal(metav1.ConditionTrue))
			g.Expect(vm.Status.RestartAwaitingChanges).To(BeNil())
		}).WithTimeout(restartConsistencyDuration).WithPolling(restartConsistencyPolling).Should(Succeed())

		By("Waiting for disk to be attached")
		Eventually(func(g Gomega) {
			g.Expect(f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)).To(Succeed())
			g.Expect(util.IsVDAttached(vm, vdBlank)).To(BeTrue())
		}).WithTimeout(framework.LongTimeout).WithPolling(hotplugPolling).Should(Succeed())

		By("Verifying disk count increased inside the guest")
		expected := initialDiskCount + 1
		Eventually(func(g Gomega) {
			count, sshErr := util.GetDiskCount(f, vm.Name, vm.Namespace)
			g.Expect(sshErr).NotTo(HaveOccurred())
			g.Expect(count).To(Equal(expected),
				"expected %d block devices in guest after hotplug, got %d", expected, count)
		}).WithTimeout(framework.MiddleTimeout).WithPolling(hotplugPolling).Should(Succeed())
	})
})

var _ = Describe("VirtualMachineBlockDeviceHotplugDetach", Label(precheck.NoPrecheck), func() {
	f := framework.NewFramework("vm-bd-hotplug-detach")

	var (
		vm               *v1alpha2.VirtualMachine
		vdRoot           *v1alpha2.VirtualDisk
		vdBlank          *v1alpha2.VirtualDisk
		initialDiskCount int
	)

	BeforeEach(func() {
		DeferCleanup(f.After)
		f.Before()
		vm, vdRoot, vdBlank, initialDiskCount = setupVM(f, true)
	})

	It("should unplug a disk without restart", func() {
		By("Removing blank disk from spec.blockDeviceRefs")
		err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
		Expect(err).NotTo(HaveOccurred())

		vm.Spec.BlockDeviceRefs = []v1alpha2.BlockDeviceSpecRef{
			{
				Kind: v1alpha2.DiskDevice,
				Name: vdRoot.Name,
			},
		}
		err = f.Clients.GenericClient().Update(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying no restart is required")
		Consistently(func(g Gomega) {
			g.Expect(f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)).To(Succeed())
			needRestart, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
			g.Expect(needRestart.Status).NotTo(Equal(metav1.ConditionTrue))
			g.Expect(vm.Status.RestartAwaitingChanges).To(BeNil())
		}).WithTimeout(restartConsistencyDuration).WithPolling(restartConsistencyPolling).Should(Succeed())

		By("Waiting for disk to be detached")
		Eventually(func(g Gomega) {
			g.Expect(f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)).To(Succeed())
			g.Expect(util.IsVDAttached(vm, vdBlank)).To(BeFalse())
		}).WithTimeout(framework.LongTimeout).WithPolling(hotplugPolling).Should(Succeed())

		By("Verifying disk count decreased inside the guest")
		expected := initialDiskCount - 1
		Eventually(func(g Gomega) {
			count, sshErr := util.GetDiskCount(f, vm.Name, vm.Namespace)
			g.Expect(sshErr).NotTo(HaveOccurred())
			g.Expect(count).To(Equal(expected),
				"expected %d block devices in guest after unplug, got %d", expected, count)
		}).WithTimeout(framework.MiddleTimeout).WithPolling(hotplugPolling).Should(Succeed())
	})
})

func setupVM(f *framework.Framework, withBlank bool) (
	vm *v1alpha2.VirtualMachine, vdRoot, vdBlank *v1alpha2.VirtualDisk, initialDiskCount int,
) {
	vdRoot = vdbuilder.New(
		vdbuilder.WithName("vd-root"),
		vdbuilder.WithNamespace(f.Namespace().Name),
		vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLAlpineBIOS,
		}),
	)

	vdBlank = vdbuilder.New(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(f.Namespace().Name),
		vdbuilder.WithSize(ptr.To(resource.MustParse("100Mi"))),
	)

	refs := []v1alpha2.BlockDeviceSpecRef{
		{Kind: v1alpha2.DiskDevice, Name: vdRoot.Name},
	}
	if withBlank {
		refs = append(refs, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: vdBlank.Name,
		})
	}

	vm = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(f.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To("100%")),
		vmbuilder.WithMemory(resource.MustParse("256Mi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
		vmbuilder.WithBlockDeviceRefs(refs...),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
	)

	err := f.CreateWithDeferredDeletion(context.Background(), vm, vdRoot, vdBlank)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for SSH to be ready")
	util.UntilSSHReady(f, vm, framework.LongTimeout)

	By("Waiting for 'lsblk' to be ready for use")
	util.UntilGuestCommandsReady(f, vm, []string{"lsblk"}, framework.MiddleTimeout)

	By("Recording initial disk count")
	initialDiskCount, err = util.GetDiskCount(f, vm.Name, vm.Namespace)
	Expect(err).NotTo(HaveOccurred())
	return vm, vdRoot, vdBlank, initialDiskCount
}
