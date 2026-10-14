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

package step

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

// legacyAllocator is the vmip allocator; with the sdn module serving the Main
// network it must not be asked at all.
type legacyAllocator struct{ asked bool }

func (a *legacyAllocator) GetAllocatedIPs(context.Context) (ip.AllocatedIPs, error) {
	a.asked = true
	return ip.AllocatedIPs{}, nil
}

func (a *legacyAllocator) AllocateNewIP(ip.AllocatedIPs) (string, error) {
	a.asked = true
	return "10.66.10.200", nil
}

func (a *legacyAllocator) IsInsideOfRange(string) error { return nil }

var _ = Describe("CreateLeaseStep", func() {
	const (
		namespace = "ns"
		address   = "10.66.10.14"
	)

	var (
		ctx       = context.Background()
		cb        *conditions.ConditionBuilder
		recorder  *eventrecord.EventRecorderLoggerMock
		allocator *legacyAllocator
		vmip      *v1alpha2.VirtualMachineIPAddress
	)

	BeforeEach(func() {
		cb = conditions.NewConditionBuilder(vmipcondition.BoundType)
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(client.Object, string, string, string) {},
		}
		allocator = &legacyAllocator{}
		vmip = &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "vmip", Namespace: namespace, UID: "vmip-uid"},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
		}
	})

	// newAutoMirror is the mirror the sdn module has already given an address.
	newAutoMirror := func(allocatedAddress string) *sdnapi.IPAddress {
		return &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmip.Name,
				Namespace:   namespace,
				Annotations: map[string]string{annotations.AnnMirrorOfVMIP: string(vmip.UID)},
			},
			Spec: sdnapi.IPAddressSpec{
				NetworkRef: sdnapi.NetworkRef{Kind: v1alpha2.NetworksTypeClusterNetwork, Name: commonnetwork.MainClusterNetworkName},
				Type:       sdnapi.IPAddressTypeAuto,
			},
			Status: sdnapi.IPAddressStatus{
				Address:    allocatedAddress,
				Conditions: []metav1.Condition{{Type: sdnapi.ConditionAllocated, Status: metav1.ConditionTrue, Reason: "IPAddressAllocated"}},
			},
		}
	}

	take := func(cl client.Client) {
		_, err := NewCreateLeaseStep(nil, allocator, cl, cb, recorder).Take(ctx, vmip)
		Expect(err).NotTo(HaveOccurred())
	}

	listLeases := func(cl client.Client) []v1alpha2.VirtualMachineIPAddressLease {
		list := &v1alpha2.VirtualMachineIPAddressLeaseList{}
		Expect(cl.List(ctx, list)).To(Succeed())
		return list.Items
	}

	It("asks the sdn module for the address: creates an Auto mirror and waits", func() {
		cl := newStepFakeClient(newMainClusterNetworkWithPool())

		take(cl)

		mirror := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: vmip.Name, Namespace: namespace}, mirror)).To(Succeed())
		Expect(mirror.Spec.Type).To(Equal(sdnapi.IPAddressTypeAuto))
		Expect(mirror.Spec.Static).To(BeNil())
		Expect(commonnetwork.IsMirrorOf(mirror, vmip)).To(BeTrue())
		Expect(listLeases(cl)).To(BeEmpty())
		Expect(allocator.asked).To(BeFalse())
		Expect(cb.Condition().Reason).To(Equal(vmipcondition.VirtualMachineIPAddressLeaseNotReady.String()))
	})

	It("takes the address the sdn module allocated to the mirror", func() {
		cl := newStepFakeClient(newMainClusterNetworkWithPool(), newAutoMirror(address))

		take(cl)

		leases := listLeases(cl)
		Expect(leases).To(HaveLen(1))
		Expect(leases[0].Name).To(Equal(ip.IPToLeaseName(address)))
		Expect(allocator.asked).To(BeFalse())
	})

	It("gives the address back when a vmip without a mirror already leases it", func() {
		// The sdn module cannot see such a lease; the next mirror gets another address.
		leaseOfAnotherVMIP := &v1alpha2.VirtualMachineIPAddressLease{ObjectMeta: metav1.ObjectMeta{Name: ip.IPToLeaseName(address)}}
		cl := newStepFakeClient(newMainClusterNetworkWithPool(), newAutoMirror(address), leaseOfAnotherVMIP)

		take(cl)

		Expect(listLeases(cl)).To(HaveLen(1))
		err := cl.Get(ctx, types.NamespacedName{Name: vmip.Name, Namespace: namespace}, &sdnapi.IPAddress{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "the mirror of a leased address must be deleted")
	})

	It("keeps the mirror when the lease of its address is this vmip's own", func() {
		// Created on the previous reconcile; the lease lookup of this one ran on a stale cache.
		leaseOfThisVMIP := &v1alpha2.VirtualMachineIPAddressLease{
			ObjectMeta: metav1.ObjectMeta{Name: ip.IPToLeaseName(address)},
			Spec: v1alpha2.VirtualMachineIPAddressLeaseSpec{
				VirtualMachineIPAddressRef: &v1alpha2.VirtualMachineIPAddressLeaseIpAddressRef{Name: vmip.Name, Namespace: namespace},
			},
		}
		cl := newStepFakeClient(newMainClusterNetworkWithPool(), newAutoMirror(address), leaseOfThisVMIP)

		take(cl)

		Expect(cl.Get(ctx, types.NamespacedName{Name: vmip.Name, Namespace: namespace}, &sdnapi.IPAddress{})).To(Succeed())
	})

	It("allocates by the vmip allocator while a vmip address is not taken in the sdn pool", func() {
		vmipWithoutMirror := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: namespace},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: address},
		}
		cl := newStepFakeClient(newMainClusterNetworkWithPool(), vmipWithoutMirror)

		take(cl)

		Expect(allocator.asked).To(BeTrue())
		Expect(listLeases(cl)).To(HaveLen(1))
		ipas := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, ipas)).To(Succeed())
		Expect(ipas.Items).To(BeEmpty())
	})

	It("creates an Auto IPAddress once every vmip address is taken in the sdn pool", func() {
		vmipWithMirror := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: namespace},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: address},
		}
		cl := newStepFakeClient(newMainClusterNetworkWithPool(), vmipWithMirror, newAllocatedIPAddress("old", address))

		take(cl)

		Expect(allocator.asked).To(BeFalse())
		Expect(cl.Get(ctx, types.NamespacedName{Name: vmip.Name, Namespace: namespace}, &sdnapi.IPAddress{})).To(Succeed())
	})

	It("takes the address of its Auto IPAddress while a vmip address is not taken in the sdn pool", func() {
		const mirrorAddress = "10.66.10.15"
		vmipWithoutMirror := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: namespace},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: address},
		}
		cl := newStepFakeClient(newMainClusterNetworkWithPool(), vmipWithoutMirror, newAutoMirror(mirrorAddress))

		take(cl)

		Expect(allocator.asked).To(BeFalse())
		leases := listLeases(cl)
		Expect(leases).To(HaveLen(1))
		Expect(leases[0].Name).To(Equal(ip.IPToLeaseName(mirrorAddress)))
	})

	It("reports an exhausted pool", func() {
		mirror := newAutoMirror("")
		mirror.Status.Conditions = []metav1.Condition{{Type: sdnapi.ConditionAllocated, Status: metav1.ConditionFalse, Reason: sdnapi.ReasonNoFreeIPAddress}}
		cl := newStepFakeClient(newMainClusterNetworkWithPool(), mirror)

		take(cl)

		Expect(listLeases(cl)).To(BeEmpty())
		Expect(cb.Condition().Message).To(ContainSubstring("No free IP addresses"))
	})

	It("keeps the vmip allocator where the sdn module does not serve the Main network", func() {
		cl := newStepFakeClient()

		take(cl)

		Expect(allocator.asked).To(BeTrue())
		Expect(listLeases(cl)).To(HaveLen(1))
		ipas := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, ipas)).To(Succeed())
		Expect(ipas.Items).To(BeEmpty())
	})

	It("allocates a Static vmip by the vmip allocator", func() {
		vmip.Spec.Type = v1alpha2.VirtualMachineIPAddressTypeStatic
		vmip.Spec.StaticIP = address
		cl := newStepFakeClient(newMainClusterNetworkWithPool())

		take(cl)

		Expect(listLeases(cl)).To(HaveLen(1))
		Expect(allocator.asked).To(BeTrue())
	})
})
