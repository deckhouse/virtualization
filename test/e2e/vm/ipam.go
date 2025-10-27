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
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmipoption "github.com/deckhouse/virtualization-controller/pkg/builder/vmip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
)

var _ = Describe("IPAM", framework.CommonE2ETestDecorators(), func() {
	var (
		f   = framework.NewFramework("ipam")
		ctx context.Context
	)

	BeforeEach(func() {
		f.Before()
		ctx = context.Background()
	})

	AfterEach(func() {
		f.After()
	})

	Context("vmip with type Auto", func() {
		It("Creates vmip with type Auto", func() {
			By("Create the auto vmip and check its binding with a lease")
			vmipAuto := object.NewVirtualMachineIPAddress("vmip-auto", f.Namespace().Name, vmipoption.WithTypeAuto())

			err := f.CreateWithDeferredDeletion(ctx, vmipAuto)
			Expect(err).NotTo(HaveOccurred())

			var lease *v1alpha2.VirtualMachineIPAddressLease
			vmipAuto, lease = WaitToBeBound(ctx, f, vmipAuto.Name)

			By("Remove labels from the lease")
			patch, err := json.Marshal([]map[string]interface{}{{
				"op":   "remove",
				"path": "/metadata/labels",
			}})
			Expect(err).NotTo(HaveOccurred())
			_, err = f.Clients.VirtClient().VirtualMachineIPAddressLeases().Patch(ctx, lease.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Wait for the label to be restored by the controller")
			_, _ = WaitToBeBound(ctx, f, vmipAuto.Name)
		})
	})

	Context("vmip with type Static", func() {
		It("Creates vmip with type Static", func() {
			By("Create an intermediate vmip to allocate a new ip address")
			intermediate := object.NewVirtualMachineIPAddress("vmip-intermediate", f.Namespace().Name, vmipoption.WithTypeAuto())
			err := f.CreateWithDeferredDeletion(ctx, intermediate)
			Expect(err).NotTo(HaveOccurred())

			var lease *v1alpha2.VirtualMachineIPAddressLease
			intermediate, lease = WaitToBeBound(ctx, f, intermediate.Name)

			By("Delete the intermediate vmip and check that the lease is released")
			err = f.Delete(ctx, intermediate)
			Expect(err).NotTo(HaveOccurred())
			lease = WaitForLeaseToBeReleased(ctx, f, lease.Name)

			By("Reuse the released lease with a static vmip")
			vmipStatic := object.NewVirtualMachineIPAddress(
				"vmip-static",
				f.Namespace().Name,
				vmipoption.WithTypeStatic(intermediate.Status.Address),
			)

			err = f.CreateWithDeferredDeletion(ctx, vmipStatic)
			Expect(err).NotTo(HaveOccurred())
			WaitToBeBound(ctx, f, vmipStatic.Name)

			By("Delete the static vmip and lease, then create another static vmip with this ip address")
			err = f.Delete(ctx, vmipStatic, lease)
			Expect(err).NotTo(HaveOccurred())

			vmipStatic = object.NewVirtualMachineIPAddress(
				"vmip-one-more-static",
				f.Namespace().Name,
				vmipoption.WithTypeStatic(intermediate.Status.Address),
			)
			err = f.CreateWithDeferredDeletion(ctx, vmipStatic)
			Expect(err).NotTo(HaveOccurred())
			WaitToBeBound(ctx, f, vmipStatic.Name)
		})
	})
})

func ExpectToBeReleased(g Gomega, lease *v1alpha2.VirtualMachineIPAddressLease) {
	boundCondition, _ := conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
	g.Expect(boundCondition.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(boundCondition.Reason).To(Equal(vmiplcondition.Released.String()))
	g.Expect(boundCondition.ObservedGeneration).To(Equal(lease.Generation))
	g.Expect(lease.Status.Phase).To(Equal(v1alpha2.VirtualMachineIPAddressLeasePhaseReleased))
}

func ExpectToBeBound(g Gomega, vmip *v1alpha2.VirtualMachineIPAddress, lease *v1alpha2.VirtualMachineIPAddressLease) {
	// 1. Check vmip to be Bound.
	boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, vmip.Status.Conditions)
	g.Expect(boundCondition.Status).To(Equal(metav1.ConditionTrue), "vmip status is not bound")
	g.Expect(boundCondition.Reason).To(Equal(vmipcondition.Bound.String()), "vmip  reason is not bound")
	g.Expect(boundCondition.ObservedGeneration).To(Equal(vmip.Generation), "vmip observed generation is not equal")

	g.Expect(vmip.Status.Phase).To(Equal(v1alpha2.VirtualMachineIPAddressPhaseBound), "phase is not bound")
	g.Expect(vmip.Status.Address).NotTo(BeEmpty(), "vmip.Status.Address is empty")
	g.Expect(ipAddressToLeaseName(vmip.Status.Address)).To(Equal(lease.Name), "lease name is not equal")

	// 2. Check lease to be Bound.
	boundCondition, _ = conditions.GetCondition(vmiplcondition.BoundType, lease.Status.Conditions)
	g.Expect(boundCondition.Status).To(Equal(metav1.ConditionTrue), "lease status is not bound")
	g.Expect(boundCondition.Reason).To(Equal(vmiplcondition.Bound.String()), "lease reason is not bound")
	g.Expect(boundCondition.ObservedGeneration).To(Equal(lease.Generation), "lease observed generation is not equal")

	g.Expect(lease.Status.Phase).To(Equal(v1alpha2.VirtualMachineIPAddressLeasePhaseBound))
	g.Expect(lease.Labels["virtualization.deckhouse.io/virtual-machine-ip-address-uid"]).To(Equal(string(vmip.UID)), "lease label is not equal")
	g.Expect(lease.Spec.VirtualMachineIPAddressRef).NotTo(BeNil(), "lease spec.VirtualMachineIPAddressRef is nil")
	g.Expect(lease.Spec.VirtualMachineIPAddressRef.Name).To(Equal(vmip.Name), "lease spec.VirtualMachineIPAddressRef.Name is not equal")
	g.Expect(lease.Spec.VirtualMachineIPAddressRef.Namespace).To(Equal(vmip.Namespace), "lease spec.VirtualMachineIPAddressRef.Namespace is not equal")
}

func WaitToBeBound(ctx context.Context, f *framework.Framework, vmipName string) (*v1alpha2.VirtualMachineIPAddress, *v1alpha2.VirtualMachineIPAddressLease) {
	var vmip *v1alpha2.VirtualMachineIPAddress
	var lease *v1alpha2.VirtualMachineIPAddressLease

	GinkgoHelper()
	Eventually(func(g Gomega) {
		var err error
		vmip, err = f.VirtClient().VirtualMachineIPAddresses(f.Namespace().Name).Get(ctx, vmipName, metav1.GetOptions{})
		g.Expect(err).NotTo(HaveOccurred())

		lease, err = f.VirtClient().VirtualMachineIPAddressLeases().Get(ctx, ipAddressToLeaseName(vmip.Status.Address), metav1.GetOptions{})
		g.Expect(err).NotTo(HaveOccurred())

		ExpectToBeBound(g, vmip, lease)
	}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())

	return vmip, lease
}

func WaitForLeaseToBeReleased(ctx context.Context, f *framework.Framework, leaseName string) *v1alpha2.VirtualMachineIPAddressLease {
	var lease *v1alpha2.VirtualMachineIPAddressLease

	GinkgoHelper()
	Eventually(func(g Gomega) {
		var err error
		lease, err = f.VirtClient().VirtualMachineIPAddressLeases().Get(ctx, leaseName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		ExpectToBeReleased(g, lease)
	}).WithTimeout(framework.ShortTimeout).WithPolling(time.Second).Should(Succeed())

	return lease
}

func ipAddressToLeaseName(ipAddress string) string {
	return "ip-" + strings.ReplaceAll(ipAddress, ".", "-")
}
