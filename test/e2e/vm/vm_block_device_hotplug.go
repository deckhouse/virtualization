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
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const hotplugPolling = 5 * time.Second

// requireNoRestart reports an invariant violation when the VM starts awaiting
// a restart to apply configuration: a hotplug must apply live. Registered with
// [vmobs.Observer.Never] right after the hotplug spec change, it is enforced
// against every VM update through the end of the spec.
func requireNoRestart() vmobs.Predicate {
	return func(m *v1alpha2.VirtualMachine) (bool, error) {
		needRestart, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, m.Status.Conditions)
		if needRestart.Status == metav1.ConditionTrue || m.Status.RestartAwaitingChanges != nil {
			return true, fmt.Errorf("the VM awaits a restart to apply configuration: %s", needRestart.Message)
		}
		return false, nil
	}
}

var _ = Describe("VirtualMachineBlockDeviceHotplugAttach", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	f := framework.NewFramework("vm-block-device-hotplug-attach")

	var (
		vm               *v1alpha2.VirtualMachine
		vdBlank          *v1alpha2.VirtualDisk
		vmObs            vmobs.Observer
		initialDiskCount int
	)

	BeforeEach(func() {
		DeferCleanup(f.After)
		f.Before()
		vm, _, vdBlank, vmObs, initialDiskCount = setupVM(f, false)
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

		// A hotplug must not require a restart: enforce it as an invariant on
		// every VM update from here through the end of the spec (a stronger
		// guarantee than the former bounded Consistently window).
		vmObs.Never(requireNoRestart())

		By("Waiting for disk to be attached")
		err = vmObs.WaitFor(vmobs.HaveBlockDevicesAttached(vdBlank.Name), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying disk count increased inside the guest")
		eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace,
			Equal(initialDiskCount+1), framework.LongTimeout,
			eventually.WithPolling(hotplugPolling),
			eventually.WithExplanation("expected %d block devices in guest after hotplug", initialDiskCount+1))
	})
})

var _ = Describe("VirtualMachineBlockDeviceHotplugDetach", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	f := framework.NewFramework("vm-block-device-hotplug-detach")

	var (
		vm               *v1alpha2.VirtualMachine
		vdRoot           *v1alpha2.VirtualDisk
		vdBlank          *v1alpha2.VirtualDisk
		vmObs            vmobs.Observer
		initialDiskCount int
	)

	BeforeEach(func() {
		DeferCleanup(f.After)
		f.Before()
		vm, vdRoot, vdBlank, vmObs, initialDiskCount = setupVM(f, true)
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

		// An unplug must not require a restart: enforce it as an invariant on
		// every VM update from here through the end of the spec (a stronger
		// guarantee than the former bounded Consistently window).
		vmObs.Never(requireNoRestart())

		By("Waiting for disk to be detached")
		err = vmObs.WaitFor(vmobs.HaveBlockDeviceDetached(vdBlank.Name), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Verifying disk count decreased inside the guest")
		eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace,
			Equal(initialDiskCount-1), framework.LongTimeout,
			eventually.WithPolling(hotplugPolling),
			eventually.WithExplanation("expected %d block devices in guest after unplug", initialDiskCount-1))
	})
})

func setupVM(f *framework.Framework, withBlank bool) (
	vm *v1alpha2.VirtualMachine, vdRoot, vdBlank *v1alpha2.VirtualDisk, vmObs vmobs.Observer, initialDiskCount int,
) {
	vdRoot = object.NewVD(
		vdbuilder.WithName("vd-root"),
		vdbuilder.WithNamespace(f.Namespace().Name),
		vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLCustomBIOS,
		}),
	)

	vdBlank = object.NewVD(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(f.Namespace().Name),
		vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
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
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		// The custom image has no cloud-init; the guest agent is baked
		// into the image, so no provisioning is needed.
		vmbuilder.WithBlockDeviceRefs(refs...),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
	)

	vmObs = vmobs.StartObserver(context.Background(), f, vm)
	vmObs.Never(vmobs.BeFailed())

	err := f.CreateWithDeferredDeletion(context.Background(), vm, vdRoot, vdBlank)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for SSH to be ready")
	eventually.SSHReadyAsRoot(f, vm, framework.LongTimeout)
	// lsblk is baked into the custom image (util-linux), so no wait for
	// cloud-init to install it is needed.

	By("Recording initial disk count")
	initialDiskCount, err = util.GetDiskCountAsRoot(f, vm.Name, vm.Namespace)
	Expect(err).NotTo(HaveOccurred())
	return vm, vdRoot, vdBlank, vmObs, initialDiskCount
}
