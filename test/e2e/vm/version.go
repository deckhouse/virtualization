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

var _ = Describe("VirtualMachineVersions", func() {
	var f *framework.Framework

	BeforeEach(func() {
		f = framework.NewFramework("vm-versions")
		DeferCleanup(f.After)
		f.Before()
	})

	It("should expose qemu and libvirt versions in VM status", func() {
		By("Creating VirtualDisk from precreated ClusterVirtualImage")
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIAlpineUEFI,
			vdbuilder.WithSize(ptr.To(resource.MustParse("512Mi"))),
		)

		By("Creating VirtualMachine")
		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			vmbuilder.WithBootloader(v1alpha2.EFI),
			vmbuilder.WithCPU(1, ptr.To("50%")),
			vmbuilder.WithMemory(resource.MustParse("256Mi")),
			vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.DiskDevice,
					Name: vdRoot.Name,
				},
			),
		)

		By("Creating resources")
		err := f.CreateWithDeferredDeletion(context.Background(), vdRoot, vm)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VirtualMachine to be Running")
		util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vm)

		By("Checking VM status has qemu and libvirt versions")
		Eventually(func(g Gomega) {
			err := f.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(vm.Status.Versions).NotTo(BeNil())
			g.Expect(vm.Status.Versions.Qemu).NotTo(BeEmpty())
			g.Expect(vm.Status.Versions.Libvirt).NotTo(BeEmpty())
		}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
	})
})
