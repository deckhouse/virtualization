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
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualDiskProvisioning", Label(precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vd-provisioning")
		sc := framework.GetConfig().StorageClass.TemplateStorageClass
		if sc != nil && sc.Provisioner == framework.NFS {
			Skip("VirtualImages on PVC only work with block storage classes, skipping NFS")
		}

		f.Before()
		DeferCleanup(f.After)
	})

	It("verifies that a VirtualDisk is provisioned successfully from a VirtualImage on a PVC", func() {
		var (
			vi *v1alpha2.VirtualImage
			vd *v1alpha2.VirtualDisk
			vm *v1alpha2.VirtualMachine
		)

		By("Creating VirtualImage from precreated CVI", func() {
			vi = object.NewGeneratedVIFromCVI("vi-", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS)

			err := f.CreateWithDeferredDeletion(ctx, vi)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for VirtualImage to be ready", func() {
			util.UntilObjectPhase(ctx, string(v1alpha2.ImageReady), framework.LongTimeout, vi)
		})

		By("Creating VirtualDisk", func() {
			vd = object.NewVDFromVI("vd", f.Namespace().Name, vi, vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))))

			err := f.CreateWithDeferredDeletion(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualMachine and waiting for VirtualMachine to be running", func() {
			vm = object.NewMinimalVM("vm-", f.Namespace().Name, vmbuilder.WithBlockDeviceRefs(
				v1alpha2.BlockDeviceSpecRef{
					Kind: v1alpha2.VirtualDiskKind,
					Name: vd.Name,
				},
			))

			err := f.CreateWithDeferredDeletion(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
		})

		By("Waiting for guest agent to be ready", func() {
			util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Waiting for VirtualDisk to be ready", func() {
			util.UntilObjectPhase(ctx, string(v1alpha2.DiskReady), framework.LongTimeout, vd)
		})
	})

	It("verifies that a VirtualDisk is provisioned successfully from a VirtualImage on dvcr", func() {
		var (
			vi *v1alpha2.VirtualImage
			vd *v1alpha2.VirtualDisk
			vm *v1alpha2.VirtualMachine
		)
		By("Creating VirtualImage", func() {
			vi = object.NewGeneratedVIFromCVI("vi-", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS)
			err := f.CreateWithDeferredDeletion(ctx, vi)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for VirtualImage to be ready", func() {
			util.UntilObjectPhase(ctx, string(v1alpha2.ImageReady), framework.LongTimeout, vi)
		})

		By("Creating VirtualDisk", func() {
			vd = object.NewVDFromVI("vd", f.Namespace().Name, vi, vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))))
			err := f.CreateWithDeferredDeletion(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualMachine and waiting for VirtualMachine to be running", func() {
			vm = object.NewMinimalVM("vm-", f.Namespace().Name, vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}))
			err := f.CreateWithDeferredDeletion(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
		})

		By("Waiting for guest agent to be ready", func() {
			util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Waiting for VirtualDisk to be ready", func() {
			util.UntilObjectPhase(ctx, string(v1alpha2.DiskReady), framework.LongTimeout, vd)
		})
	})

	It("verifies that a VirtualDisk is provisioned successfully from a ClusterVirtualImage", func() {
		var (
			vd *v1alpha2.VirtualDisk
			vm *v1alpha2.VirtualMachine
		)

		By("Creating VirtualDisk", func() {
			vd = object.NewVDFromCVI("vd", f.Namespace().Name, object.PrecreatedCVIAlpineBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))))
			err := f.CreateWithDeferredDeletion(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualMachine and waiting for VirtualMachine to be running", func() {
			vm = object.NewMinimalVM("vm-", f.Namespace().Name, vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}))
			err := f.CreateWithDeferredDeletion(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
		})

		By("Waiting for guest agent to be ready", func() {
			util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Waiting for VirtualDisk to be ready", func() {
			util.UntilObjectPhase(ctx, string(v1alpha2.DiskReady), framework.LongTimeout, vd)
		})
	})

	It("verifies that a VirtualDisk is provisioned successfully from a http", func() {
		var (
			vd *v1alpha2.VirtualDisk
			vm *v1alpha2.VirtualMachine
		)

		By("Creating VirtualDisk", func() {
			vd = object.NewHTTPVDAlpineBIOS("vd", f.Namespace().Name, vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))))
			err := f.CreateWithDeferredDeletion(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating VirtualMachine and waiting for VirtualMachine to be running", func() {
			vm = object.NewMinimalVM("vm-", f.Namespace().Name, vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vd.Name,
			}))
			err := f.CreateWithDeferredDeletion(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)
		})

		By("Waiting for guest agent to be ready", func() {
			util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Waiting for VirtualDisk to be ready", func() {
			util.UntilObjectPhase(ctx, string(v1alpha2.DiskReady), framework.LongTimeout, vd)
		})
	})
})
