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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const evacuationAnnotation = "virtualization.deckhouse.io/evacuation"

var _ = Describe("VirtualMachineEvacuation", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var f *framework.Framework

	BeforeEach(func() {
		f = framework.NewFramework("vm-evacuation")
		DeferCleanup(f.After)
		f.Before()
	})

	It("evacuates virtual machines after active pod eviction", func() {
		ctx := context.Background()

		By("Environment preparation")
		vmBIOS, vdRootBIOS, vdBlankBIOS := newEvacuationVM(
			"vm-evacuation-bios",
			f.Namespace().Name,
			object.PrecreatedCVICustomBIOS,
			vdCustomImageSize,
			v1alpha2.BIOS,
			// The custom image has no cloud-init; the guest agent is baked in.
		)
		vmUEFI, vdRootUEFI, vdBlankUEFI := newEvacuationVM(
			"vm-evacuation-uefi",
			f.Namespace().Name,
			object.PrecreatedCVICustomEFI,
			vdCustomImageSize,
			v1alpha2.EFI,
			// The custom image has no cloud-init; the guest agent is baked in.
			// OVMF does not fit the BIOS sizing NewMinimalVM defaults to.
			vmbuilder.WithMemory(resource.MustParse(object.CustomImageEFIVMMemory)),
		)

		// Arm the observers before creating the VMs so no transition is missed.
		vmBIOSObs := vmobs.StartObserver(ctx, f, vmBIOS)
		vmUEFIObs := vmobs.StartObserver(ctx, f, vmUEFI)

		err := f.CreateWithDeferredDeletion(
			ctx,
			vdRootBIOS, vdBlankBIOS, vmBIOS,
			vdRootUEFI, vdBlankUEFI, vmUEFI,
		)
		Expect(err).NotTo(HaveOccurred())

		err = vmBIOSObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		err = vmUEFIObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Evacuate virtual machines by active pod eviction")
		evacuateVirtualMachines(ctx, f, vmBIOS, vmUEFI)

		By("Waiting for evacuation VMOPs to finish")
		// The evacuation VMOPs are created asynchronously by the controller with
		// generated names, so each is discovered by watching the namespace's
		// VMOPs for a finished evacuation operation of the VM.
		for _, vm := range []*v1alpha2.VirtualMachine{vmBIOS, vmUEFI} {
			_, err := observer.WaitForFirst(ctx,
				f.VirtClient().VirtualMachineOperations(f.Namespace().Name),
				framework.LongTimeout,
				func(vmop *v1alpha2.VirtualMachineOperation) bool {
					if vmop.Spec.VirtualMachine != vm.Name {
						return false
					}
					if _, exists := vmop.Annotations[evacuationAnnotation]; !exists {
						return false
					}
					switch vmop.Status.Phase {
					case v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseCompleted, v1alpha2.VMOPPhaseSuperseded:
						return true
					default:
						return false
					}
				})
			Expect(err).NotTo(HaveOccurred(), "evacuation VMOP for VM %s should finish", vm.Name)
		}
	})
})

func newEvacuationVM(name, namespace, cviName, rootSize string, bootloader v1alpha2.BootloaderType, extraVMOpts ...vmbuilder.Option) (
	*v1alpha2.VirtualMachine,
	*v1alpha2.VirtualDisk,
	*v1alpha2.VirtualDisk,
) {
	// Long disk names (>60 chars, the former limit) to exercise live-migrating a VM
	// whose disks use the full Kubernetes name length. The VM name itself stays
	// short, as VirtualMachine names remain limited.
	longSuffix := "-" + strings.Repeat("a", 80)
	vdRoot := object.NewVDFromCVI(
		name+"-root"+longSuffix,
		namespace,
		cviName,
		vdbuilder.WithSize(ptr.To(resource.MustParse(rootSize))),
	)

	vdBlank := object.NewBlankVD(
		name+"-blank"+longSuffix,
		namespace,
		nil,
		ptr.To(resource.MustParse(vdCustomImageSize)),
	)

	opts := append([]vmbuilder.Option{
		vmbuilder.WithName(name),
		vmbuilder.WithBootloader(bootloader),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdBlank.Name,
			},
		),
	}, extraVMOpts...)

	vm := object.NewMinimalVM("", namespace, opts...)

	return vm, vdRoot, vdBlank
}

func evacuateVirtualMachines(ctx context.Context, f *framework.Framework, vms ...*v1alpha2.VirtualMachine) {
	GinkgoHelper()

	// The active virt-launcher pod carries a generated name recorded in the VM
	// status with a small lag, so observe each VM through a watch until its
	// status reports an active pod, then fetch that pod.
	var pods []corev1.Pod
	for _, vm := range vms {
		obs, err := observer.New[*v1alpha2.VirtualMachine](
			ctx,
			f.VirtClient().VirtualMachines(vm.Namespace),
			vm.Name, vm.Namespace,
		)
		Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachine %s/%s", vm.Namespace, vm.Name)

		waitErr := obs.WaitFor(vmobs.HaveActivePod(), framework.MiddleTimeout)
		obs.Stop()
		Expect(waitErr).NotTo(HaveOccurred(), "VM %s should report an active virt-launcher pod", vm.Name)

		_, pod, err := util.GetVirtualMachineAndActivePod(ctx, f, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(pod).NotTo(BeNil())
		pods = append(pods, *pod)
	}

	for _, pod := range pods {
		err := f.KubeClient().CoreV1().Pods(pod.GetNamespace()).EvictV1(ctx, &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.GetName(),
				Namespace: pod.GetNamespace(),
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("Eviction triggered evacuation of VMI")))
	}
}
