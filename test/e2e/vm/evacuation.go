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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

const evacuationAnnotation = "virtualization.deckhouse.io/evacuation"

var _ = Describe("VirtualMachineEvacuation", Label(precheck.NoPrecheck), func() {
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
			object.PrecreatedCVIAlpineBIOS,
			v1alpha2.BIOS,
		)
		vmUEFI, vdRootUEFI, vdBlankUEFI := newEvacuationVM(
			"vm-evacuation-uefi",
			f.Namespace().Name,
			object.PrecreatedCVIAlpineUEFI,
			v1alpha2.EFI,
		)

		err := f.CreateWithDeferredDeletion(
			ctx,
			vdRootBIOS, vdBlankBIOS, vmBIOS,
			vdRootUEFI, vdBlankUEFI, vmUEFI,
		)
		Expect(err).NotTo(HaveOccurred())

		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmBIOS), framework.LongTimeout)
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vmUEFI), framework.LongTimeout)

		By("Evacuate virtual machines by active pod eviction")
		evacuateVirtualMachines(ctx, f, vmBIOS, vmUEFI)

		By("Waiting for evacuation VMOPs to finish")
		vmNames := map[string]struct{}{
			vmBIOS.Name: {},
			vmUEFI.Name: {},
		}

		Eventually(func(g Gomega) {
			vmops := &v1alpha2.VirtualMachineOperationList{}
			err := f.GenericClient().List(ctx, vmops, crclient.InNamespace(f.Namespace().Name))
			g.Expect(err).NotTo(HaveOccurred())

			finishedVMOPs := make(map[string]struct{}, len(vmNames))
			for _, vmop := range vmops.Items {
				if _, exists := vmNames[vmop.Spec.VirtualMachine]; !exists {
					continue
				}
				if _, exists := vmop.Annotations[evacuationAnnotation]; !exists {
					continue
				}
				if vmop.Status.Phase == v1alpha2.VMOPPhaseFailed || vmop.Status.Phase == v1alpha2.VMOPPhaseCompleted {
					finishedVMOPs[vmop.Spec.VirtualMachine] = struct{}{}
				}
			}

			g.Expect(finishedVMOPs).To(HaveLen(len(vmNames)))
		}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())
	})
})

func newEvacuationVM(name, namespace, cviName string, bootloader v1alpha2.BootloaderType) (
	*v1alpha2.VirtualMachine,
	*v1alpha2.VirtualDisk,
	*v1alpha2.VirtualDisk,
) {
	vdRoot := object.NewVDFromCVI(
		name+"-root",
		namespace,
		cviName,
		vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
	)

	vdBlank := object.NewBlankVD(
		name+"-blank",
		namespace,
		nil,
		ptr.To(resource.MustParse("100Mi")),
	)

	vm := object.NewMinimalVM(
		"",
		namespace,
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
	)

	return vm, vdRoot, vdBlank
}

func evacuateVirtualMachines(ctx context.Context, f *framework.Framework, vms ...*v1alpha2.VirtualMachine) {
	GinkgoHelper()

	var pods []corev1.Pod
	Eventually(func(g Gomega) {
		pods = []corev1.Pod{}
		for _, vm := range vms {
			_, pod, err := util.GetVirtualMachineAndActivePod(ctx, f, vm)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pod).NotTo(BeNil())
			pods = append(pods, *pod)
		}
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())

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
