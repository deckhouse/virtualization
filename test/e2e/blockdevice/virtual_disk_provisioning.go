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

package blockdevice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("VirtualDiskProvisioning", Label(label.SIGStorage, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("virtual-disk-provisioning")

		f.Before()
		DeferCleanup(f.After)
		// This suite does not use setupProject, which normally clears the
		// scenario-node pin, so drop the namespace's entry here.
		DeferCleanup(func() {
			pinnedScenarioNodes.Delete(f.Namespace().Name)
		})
	})

	// runVMConsumingDisk creates a consumer VirtualMachine for vd and waits until
	// it is Running with a ready guest agent, then until the disk is Ready. The
	// custom image has no cloud-init, so provisioning is disabled: the VM
	// only needs to boot (its agent auto-starts) to consume the disk.
	runVMConsumingDisk := func(vd *v1alpha2.VirtualDisk, vdObs vdobs.Observer) {
		GinkgoHelper()
		vmOpts := []vmbuilder.Option{
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}),
		}
		if node, ok := scenarioNode(f); ok {
			// TODO: remove this test-level pin once local PVC/snapshot sources and
			// target VMs are guaranteed to use the same node by the controllers.
			// Without it, the disk cloned from a node-local VirtualImage PV can only
			// be provisioned on the image's node, while the VM and its hotplug pod
			// are free to start on another node and deadlock there.
			vmOpts = append(vmOpts, vmbuilder.WithNodeSelector(map[string]string{
				hostnameNodeSelectorKey: node,
			}))
		}
		vm := object.NewMinimalVM("vm-", f.Namespace().Name, vmOpts...)
		Expect(f.CreateWithDeferredDeletion(ctx, vm)).To(Succeed())

		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())
		vmObs.Never(vmobs.HaveNoBootableDevice())
		err := vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		err = vdObs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	}

	It("verifies that a VirtualDisk is provisioned successfully from a VirtualImage on a PVC", func() {
		vi := object.NewGeneratedVIFromCVI("vi-", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim))
		vi.Spec.PersistentVolumeClaim.StorageClass = defaultStorageClass()
		Expect(f.CreateWithDeferredDeletion(ctx, vi)).To(Succeed())

		viObs := viobs.StartObserver(ctx, f, vi)
		viObs.Never(viobs.BeFailed())
		err := viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		// On node-local storage the disk below is cloned from the image's PV via a
		// CSI snapshot and can only be provisioned on the image's node, so pin the
		// consumer VM there.
		rememberVirtualImageNode(ctx, f, vi)

		// No explicit size: the controller derives it from the source image, which
		// matches the clone snapshot's restoreSize (see virtual_disk_creation.go).
		vd := object.NewVDFromVI("vd", f.Namespace().Name, vi, vdbuilder.WithStorageClass(defaultStorageClass()))
		Expect(f.CreateWithDeferredDeletion(ctx, vd)).To(Succeed())
		vdObs := vdobs.StartObserver(ctx, f, vd)
		vdObs.Never(vdobs.BeFailed())

		runVMConsumingDisk(vd, vdObs)
	})

	It("verifies that a VirtualDisk is provisioned successfully from a VirtualImage on dvcr", func() {
		vi := object.NewGeneratedVIFromCVI("vi-", f.Namespace().Name, object.PrecreatedCVICustomBIOS)
		Expect(f.CreateWithDeferredDeletion(ctx, vi)).To(Succeed())

		viObs := viobs.StartObserver(ctx, f, vi)
		viObs.Never(viobs.BeFailed())
		err := viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		vd := object.NewVDFromVI("vd", f.Namespace().Name, vi, vdbuilder.WithStorageClass(defaultStorageClass()))
		Expect(f.CreateWithDeferredDeletion(ctx, vd)).To(Succeed())
		vdObs := vdobs.StartObserver(ctx, f, vd)
		vdObs.Never(vdobs.BeFailed())

		runVMConsumingDisk(vd, vdObs)
	})

	It("verifies that a VirtualDisk is provisioned successfully from a ClusterVirtualImage", func() {
		vd := object.NewVDFromCVI("vd", f.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))), vdbuilder.WithStorageClass(defaultStorageClass()))
		Expect(f.CreateWithDeferredDeletion(ctx, vd)).To(Succeed())
		vdObs := vdobs.StartObserver(ctx, f, vd)
		vdObs.Never(vdobs.BeFailed())

		runVMConsumingDisk(vd, vdObs)
	})

	It("verifies that a VirtualDisk is provisioned successfully from a http", func() {
		vd := object.NewHTTPVDCustomBIOS("vd", f.Namespace().Name, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))), vdbuilder.WithStorageClass(defaultStorageClass()))
		Expect(f.CreateWithDeferredDeletion(ctx, vd)).To(Succeed())
		vdObs := vdobs.StartObserver(ctx, f, vd)
		vdObs.Never(vdobs.BeFailed())

		runVMConsumingDisk(vd, vdObs)
	})
})
