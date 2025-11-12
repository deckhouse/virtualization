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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmsbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	vmsopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VMSOPCreateVirtualMachine", func() {
	var (
		vd         *v1alpha2.VirtualDisk
		vm         *v1alpha2.VirtualMachine
		vmsnapshot *v1alpha2.VirtualMachineSnapshot
		vmsop      *v1alpha2.VirtualMachineSnapshotOperation

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
			vm = object.NewMinimalVM("vmsop-origin-", f.Namespace().Name,
				vmbuilder.WithBlockDeviceRefs(
					v1alpha2.BlockDeviceSpecRef{
						Kind: v1alpha2.VirtualDiskKind,
						Name: vd.Name,
					},
				),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vd, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Create VM Snapshot", func() {
			vmsnapshot = vmsbuilder.New(
				vmsbuilder.WithName("vmsnapshot"),
				vmsbuilder.WithNamespace(f.Namespace().Name),
				vmsbuilder.WithVirtualMachineName(vm.Name),
				vmsbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressNever),
				vmsbuilder.WithRequiredConsistency(false),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vmsnapshot)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.VirtualMachineSnapshotPhaseReady), framework.LongTimeout, vmsnapshot)
		})

		By("Create and wait for VMSOP", func() {
			vmsop = vmsopbuilder.New(
				vmsopbuilder.WithName("vmsop"),
				vmsopbuilder.WithNamespace(f.Namespace().Name),
				vmsopbuilder.WithVirtualMachineSnapshotName(vmsnapshot.Name),
				vmsopbuilder.WithCreateVirtualMachine(&v1alpha2.VMSOPCreateVirtualMachineSpec{
					Mode: v1alpha2.VMSOPCreateVirtualMachineModeBestEffort,
					Customization: &v1alpha2.VMSOPCreateVirtualMachineCustomization{
						NamePrefix: "created-from-vmsop-",
					},
				}),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), vmsop)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.VMSOPPhaseCompleted), framework.LongTimeout, vmsop)
		})

		By("Verify that the created VM is running", func() {
			newName := fmt.Sprintf("created-from-vmsop-%s", vm.Name)
			createdVM, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).Get(context.Background(), newName, v1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(createdVM), framework.LongTimeout)
		})
	})
})
