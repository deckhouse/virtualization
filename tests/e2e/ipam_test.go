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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("IPAM", framework.CommonE2ETestDecorators(), func() {
	var (
		ns     string
		ctx    context.Context
		cancel context.CancelFunc
		vmip   *virtv2.VirtualMachineIPAddress

		virtClient = framework.GetClients().VirtClient()
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.IPAM, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred())

		CreateNamespace(ns)

		res := kubectl.Apply(kc.ApplyOptions{
			Filename:       []string{conf.TestData.IPAM},
			FilenameOption: kc.Kustomize,
		})
		Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 50*time.Second)

		vmip = &virtv2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmip",
				Namespace: ns,
			},
			Spec: virtv2.VirtualMachineIPAddressSpec{
				Type: virtv2.VirtualMachineIPAddressTypeAuto,
			},
		}
	})

	Context("vmip with type Auto", func() {
		It("Creates vmip with type Auto", func() {
			By("Create a vmip automatically and check its binding with a lease")
			vmipAuto := vmip.DeepCopy()
			vmipAuto.Name += "-auto"
			vmipAuto, lease := CreateVirtualMachineIPAddress(ctx, vmipAuto)
			ExpectToBeBound(vmipAuto, lease)

			By("Remove label from the lease")
			patch, err := json.Marshal([]map[string]interface{}{{
				"op":   "remove",
				"path": "/metadata/labels",
			}})
			Expect(err).NotTo(HaveOccurred())
			_, err = virtClient.VirtualMachineIPAddressLeases().Patch(ctx, lease.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Wait for the label to be restored by the controller")
			lease = WaitForVirtualMachineIPAddressLease(ctx, lease.Name, func(_ watch.EventType, e *virtv2.VirtualMachineIPAddressLease) (bool, error) {
				return e.Labels["virtualization.deckhouse.io/virtual-machine-ip-address-uid"] == string(vmipAuto.UID), nil
			})
			vmipAuto, err = virtClient.VirtualMachineIPAddresses(vmipAuto.Namespace).Get(ctx, vmipAuto.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			ExpectToBeBound(vmipAuto, lease)
		})
	})

	Context("vmip with type Static", func() {
		It("Creates vmip with type Static", func() {
			By("Create an intermediate vmip automatically to allocate a new ip address")
			intermediate := vmip.DeepCopy()
			intermediate.Name += "-intermediate"
			intermediate, lease := CreateVirtualMachineIPAddress(ctx, intermediate)
			ExpectToBeBound(intermediate, lease)

			By("Delete the intermediate vmip automatically and check that the lease is released")
			DeleteResource(ctx, intermediate)
			lease = WaitForVirtualMachineIPAddressLease(ctx, lease.Name, func(_ watch.EventType, e *virtv2.VirtualMachineIPAddressLease) (bool, error) {
				boundCondition, err := GetCondition(vmiplcondition.BoundType.String(), e)
				Expect(err).NotTo(HaveOccurred())
				return boundCondition.Reason == vmiplcondition.Released.String() && boundCondition.ObservedGeneration == e.Generation, nil
			})
			ExpectToBeReleased(lease)

			By("Reuse the released lease with a static vmip")
			vmipStatic := vmip.DeepCopy()
			vmipStatic.Name += "-static"
			vmipStatic.Spec.Type = virtv2.VirtualMachineIPAddressTypeStatic
			vmipStatic.Spec.StaticIP = intermediate.Status.Address
			vmipStatic, lease = CreateVirtualMachineIPAddress(ctx, vmipStatic)
			ExpectToBeBound(vmipStatic, lease)

			By("Delete the static vmip and lease, then create another static vmip with this ip address")

			wait := make(chan struct{})
			go func() {
				defer close(wait)
				defer GinkgoRecover()
				WaitForVirtualMachineIPAddressLease(ctx, lease.Name, func(eType watch.EventType, _ *virtv2.VirtualMachineIPAddressLease) (bool, error) {
					return eType == watch.Deleted, nil
				})
			}()

			DeleteResource(ctx, vmipStatic)
			DeleteResource(ctx, lease)
			// Wait for the lease to be deleted.
			<-wait

			vmipStatic = vmip.DeepCopy()
			vmipStatic.Name += "-one-more-static"
			vmipStatic.Spec.Type = virtv2.VirtualMachineIPAddressTypeStatic
			vmipStatic.Spec.StaticIP = intermediate.Status.Address
			vmipStatic, lease = CreateVirtualMachineIPAddress(ctx, vmipStatic)
			ExpectToBeBound(vmipStatic, lease)
		})
	})

	AfterEach(func() {
		cancel()
	})
})

func WaitForVirtualMachineIPAddress(ctx context.Context, ns, name string, h EventHandler[*virtv2.VirtualMachineIPAddress]) *virtv2.VirtualMachineIPAddress {
	GinkgoHelper()
	vmip, err := WaitFor(ctx, framework.GetClients().VirtClient().VirtualMachineIPAddresses(ns), h, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(),
	})
	Expect(err).NotTo(HaveOccurred())
	return vmip
}

func WaitForVirtualMachineIPAddressLease(ctx context.Context, name string, h EventHandler[*virtv2.VirtualMachineIPAddressLease]) *virtv2.VirtualMachineIPAddressLease {
	GinkgoHelper()
	lease, err := WaitFor(ctx, framework.GetClients().VirtClient().VirtualMachineIPAddressLeases(), h, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(),
	})
	Expect(err).NotTo(HaveOccurred())
	return lease
}

func CreateVirtualMachineIPAddress(ctx context.Context, vmip *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddress, *virtv2.VirtualMachineIPAddressLease) {
	GinkgoHelper()

	CreateResource(ctx, vmip)
	vmip = WaitForVirtualMachineIPAddress(ctx, vmip.Namespace, vmip.Name, func(_ watch.EventType, e *virtv2.VirtualMachineIPAddress) (bool, error) {
		return e.Status.Phase == virtv2.VirtualMachineIPAddressPhaseBound, nil
	})

	lease, err := framework.GetClients().VirtClient().VirtualMachineIPAddressLeases().Get(ctx, ipAddressToLeaseName(vmip.Status.Address), metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	return vmip, lease
}

func ExpectToBeReleased(lease *virtv2.VirtualMachineIPAddressLease) {
	GinkgoHelper()

	boundCondition, err := GetCondition(vmiplcondition.BoundType.String(), lease)
	Expect(err).NotTo(HaveOccurred())
	Expect(boundCondition.Status).To(Equal(metav1.ConditionFalse))
	Expect(boundCondition.Reason).To(Equal(vmiplcondition.Released.String()))
	Expect(boundCondition.ObservedGeneration).To(Equal(lease.Generation))
	Expect(lease.Status.Phase).To(Equal(virtv2.VirtualMachineIPAddressLeasePhaseReleased))
}

func ExpectToBeBound(vmip *virtv2.VirtualMachineIPAddress, lease *virtv2.VirtualMachineIPAddressLease) {
	GinkgoHelper()

	// 1. Check vmip to be Bound.
	boundCondition, err := GetCondition(vmipcondition.BoundType.String(), vmip)
	Expect(err).NotTo(HaveOccurred())
	Expect(boundCondition.Status).To(Equal(metav1.ConditionTrue))
	Expect(boundCondition.Reason).To(Equal(vmipcondition.Bound.String()))
	Expect(boundCondition.ObservedGeneration).To(Equal(vmip.Generation))

	Expect(vmip.Status.Phase).To(Equal(virtv2.VirtualMachineIPAddressPhaseBound))
	Expect(vmip.Status.Address).NotTo(BeEmpty())
	Expect(ipAddressToLeaseName(vmip.Status.Address)).To(Equal(lease.Name))

	// 2. Check lease to be Bound.
	boundCondition, err = GetCondition(vmiplcondition.BoundType.String(), lease)
	Expect(err).NotTo(HaveOccurred())
	Expect(boundCondition.Status).To(Equal(metav1.ConditionTrue))
	Expect(boundCondition.Reason).To(Equal(vmiplcondition.Bound.String()))
	Expect(boundCondition.ObservedGeneration).To(Equal(lease.Generation))

	Expect(lease.Status.Phase).To(Equal(virtv2.VirtualMachineIPAddressLeasePhaseBound))
	Expect(lease.Labels["virtualization.deckhouse.io/virtual-machine-ip-address-uid"]).To(Equal(string(vmip.UID)))
	Expect(lease.Spec.VirtualMachineIPAddressRef).NotTo(BeNil())
	Expect(lease.Spec.VirtualMachineIPAddressRef.Name).To(Equal(vmip.Name))
	Expect(lease.Spec.VirtualMachineIPAddressRef.Namespace).To(Equal(vmip.Namespace))
}

func ipAddressToLeaseName(ipAddress string) string {
	return "ip-" + strings.ReplaceAll(ipAddress, ".", "-")
}
