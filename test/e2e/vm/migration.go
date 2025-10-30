/*
Copyright 2025 Flant JSC

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

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineMigration", func() {
	const (
		externalHost = "https://flant.com"
		httpStatusOk = "200"
	)

	var (
		vdRootBIOS  *v1alpha2.VirtualDisk
		vdBlankBIOS *v1alpha2.VirtualDisk
		vdRootUEFI  *v1alpha2.VirtualDisk
		vdBlankUEFI *v1alpha2.VirtualDisk

		vmBIOS *v1alpha2.VirtualMachine
		vmUEFI *v1alpha2.VirtualMachine

		f = framework.NewFramework("vm-migration")
	)

	BeforeEach(func() {
		DeferCleanup(f.After)

		f.Before()
	})

	It("verifies that migrations are successful", func() {
		By("Environment preparation", func() {
			vdRootBIOS = vd.New(
				vd.WithName("vd-root-bios"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("10Gi"))),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLAlpineBIOS,
				}),
			)
			vdBlankBIOS = vd.New(
				vd.WithName("vd-blank-bios"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("100Mi"))),
			)
			vdRootUEFI = vd.New(
				vd.WithName("vd-root-uefi"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("10Gi"))),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLAlpineUEFIPerf,
				}),
			)
			vdBlankUEFI = vd.New(
				vd.WithName("vd-blank-uefi"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithSize(ptr.To(resource.MustParse("100Mi"))),
			)
			vmBIOS = object.NewMinimalVM("vm-bios-", f.Namespace().Name,
				vm.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdRootBIOS.Name,
					},
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdBlankBIOS.Name,
					},
				),
				vm.WithBootloader(v1alpha2.BIOS),
				vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
			)
			vmUEFI = object.NewMinimalVM("vm-uefi-", f.Namespace().Name,
				vm.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdRootUEFI.Name,
					},
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vdBlankUEFI.Name,
					},
				),
				vm.WithBootloader(v1alpha2.EFI),
				vm.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vdRootBIOS, vdBlankBIOS, vmBIOS, vdRootUEFI, vdBlankUEFI, vmUEFI)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmBIOS), framework.LongTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmUEFI), framework.LongTimeout)
		})

		By("Create VMOP to trigger migration", func() {
			util.MigrateVirtualMachine(vmBIOS, vmop.WithGenerateName("vmop-migrate-bios-"))
			util.MigrateVirtualMachine(vmUEFI, vmop.WithGenerateName("vmop-migrate-uefi-"))
		})

		By("Wait for migration to complete", func() {
			util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(vmBIOS), framework.LongTimeout)
			util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(vmUEFI), framework.LongTimeout)
		})

		By("Check Cilium agents are properly configured for the VM", func() {
			err := network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), vmBIOS.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", vmBIOS.Name)
			err = network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), vmUEFI.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", vmUEFI.Name)
		})

		By("Check VM can reach external network", func() {
			util.CheckExternalConnectivity(f, vmBIOS.Name, externalHost, httpStatusOk)
			util.CheckExternalConnectivity(f, vmUEFI.Name, externalHost, httpStatusOk)
		})
	})
})
