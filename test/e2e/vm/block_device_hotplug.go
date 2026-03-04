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
	"fmt"
	"strconv"
	"strings"
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

var _ = Describe("VirtualMachineBlockDeviceHotplug", Ordered, func() {
	f := framework.NewFramework("vm-bd-hotplug")

	var (
		vm               *v1alpha2.VirtualMachine
		vdRoot           *v1alpha2.VirtualDisk
		vdBlank          *v1alpha2.VirtualDisk
		initialDiskCount string
	)

	BeforeAll(func() {
		DeferCleanup(f.After)
		f.Before()

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

		vm = vmbuilder.New(
			vmbuilder.WithName("vm"),
			vmbuilder.WithNamespace(f.Namespace().Name),
			vmbuilder.WithCPU(1, ptr.To("5%")),
			vmbuilder.WithMemory(resource.MustParse("256Mi")),
			vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
			vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
			vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.DiskDevice,
					Name: vdRoot.Name,
				},
			),
			vmbuilder.WithRestartApprovalMode(v1alpha2.Automatic),
		)

		err := f.CreateWithDeferredDeletion(context.Background(), vm, vdRoot, vdBlank)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM agent to be ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		By("Recording initial disk count")
		out, err := f.SSHCommand(vm.Name, vm.Namespace, "lsblk --nodeps --noheadings | wc -l")
		Expect(err).NotTo(HaveOccurred())
		initialDiskCount = strings.TrimSpace(out)
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
		Expect(util.IsRestartRequired(vm, 5*time.Second)).To(BeFalse())

		By("Waiting for disk to be attached")
		Eventually(func(g Gomega) {
			g.Expect(f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)).To(Succeed())
			g.Expect(util.IsVDAttached(vm, vdBlank)).To(BeTrue())
		}, framework.LongTimeout, 5*time.Second).Should(Succeed())

		By("Verifying disk count increased inside the guest")
		initCount, err := strconv.Atoi(initialDiskCount)
		Expect(err).NotTo(HaveOccurred())
		expected := fmt.Sprintf("%d", initCount+1)
		Eventually(func(g Gomega) {
			out, sshErr := f.SSHCommand(vm.Name, vm.Namespace, "lsblk --nodeps --noheadings | wc -l")
			g.Expect(sshErr).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal(expected))
		}, framework.MiddleTimeout, 5*time.Second).Should(Succeed())
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
		Expect(util.IsRestartRequired(vm, 5*time.Second)).To(BeFalse())

		By("Waiting for disk to be detached")
		Eventually(func(g Gomega) {
			g.Expect(f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)).To(Succeed())
			g.Expect(util.IsVDAttached(vm, vdBlank)).To(BeFalse())
		}, framework.LongTimeout, 5*time.Second).Should(Succeed())

		By("Verifying disk count decreased inside the guest")
		Eventually(func(g Gomega) {
			out, sshErr := f.SSHCommand(vm.Name, vm.Namespace, "lsblk --nodeps --noheadings | wc -l")
			g.Expect(sshErr).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal(initialDiskCount))
		}, framework.MiddleTimeout, 5*time.Second).Should(Succeed())
	})
})
