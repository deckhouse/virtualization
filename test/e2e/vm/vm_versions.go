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

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("VirtualMachineVersions", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-versions")
		DeferCleanup(f.After)
		f.Before()
	})

	It("should expose qemu and libvirt versions in VM status", func() {
		By("Generating VirtualDisk from precreated ClusterVirtualImage")
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS,
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
		)

		By("Generating VirtualMachine")
		vm := object.NewMinimalVM("vm-", f.Namespace().Name,
			// The custom image has no cloud-init; the guest agent is baked
			// in, so no provisioning is needed.
			vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.DiskDevice,
					Name: vdRoot.Name,
				},
			),
		)

		By("Creating resources")
		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vm)
		Expect(err).NotTo(HaveOccurred())

		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())

		By("Waiting for VirtualMachine to be Running")
		err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Checking VM status has qemu and libvirt versions")
		err = vmObs.WaitFor(haveQemuAndLibvirtVersions(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})
})

// haveQemuAndLibvirtVersions reports the VM status carries both the qemu and
// the libvirt version.
func haveQemuAndLibvirtVersions() vmobs.Predicate {
	return func(m *v1alpha2.VirtualMachine) (bool, error) {
		return m.Status.Versions.Qemu != "" && m.Status.Versions.Libvirt != "", nil
	}
}
