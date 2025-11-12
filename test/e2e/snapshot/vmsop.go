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

package snapshot

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VMSOPCreateVirtualMachine", func() {
	var (
		vd *v1alpha2.VirtualDisk
		vm *v1alpha2.VirtualMachine

		f = framework.NewFramework("vmsop")
	)

	BeforeEach(func() {
		DeferCleanup(f.After)

		f.Before()
	})

	It("verifies that vmsop are successful", func() {
		By("Environment preparation", func() {
			vd = vdbuilder.New(
				vdbuilder.WithName("vd-root"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithSize(ptr.To(resource.MustParse("10Gi"))),
				vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLAlpineBIOS,
				}),
			)
			vm = object.NewMinimalVM("vm-bios-", f.Namespace().Name,
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vd.Name,
					},
				),
				vmbuilder.WithBootloader(v1alpha2.BIOS),
				vmbuilder.WithLiveMigrationPolicy(v1alpha2.PreferSafeMigrationPolicy),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vd, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Create VMOP to trigger migration", func() {
			util.MigrateVirtualMachine(f, vm, vmopbuilder.WithGenerateName("vmop-migrate-bios-"))
		})

		By("Wait for migration to complete", func() {
			util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})
	})
})
