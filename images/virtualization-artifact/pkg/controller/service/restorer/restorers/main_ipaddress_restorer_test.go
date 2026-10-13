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

package restorer

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("MainIPAddressHandler", func() {
	const (
		name       = "vm-mirror"
		namespace  = "default"
		address    = "10.66.10.14"
		restoreUID = "restore-uid"
	)

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	newMirrorOwnedByVM := func() *sdnapi.IPAddress {
		return &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.VirtualMachineKind,
					Name:       "vm",
					UID:        "vm-uid",
				}},
			},
			Spec: sdnapi.IPAddressSpec{
				NetworkRef: sdnapi.NetworkRef{
					Kind: v1alpha2.NetworksTypeClusterNetwork,
					Name: network.MainClusterNetworkName,
				},
				Type:   sdnapi.IPAddressTypeStatic,
				Static: &sdnapi.StaticIPAddress{IP: address},
			},
		}
	}

	getIPAddress := func(cl client.Client) *sdnapi.IPAddress {
		ipa := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ipa)).To(Succeed())
		return ipa
	}

	listIPAddresses := func(cl client.Client) []sdnapi.IPAddress {
		ipas := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, ipas, client.InNamespace(namespace))).To(Succeed())
		return ipas.Items
	}

	It("detaches the existing mirror owned by the VM instead of recreating it", func() {
		cl, err := testutil.NewFakeClientWithObjects(newMirrorOwnedByVM())
		Expect(err).NotTo(HaveOccurred())

		handler := NewMainIPAddressHandler(cl, name, namespace, address, []string{"vm-uid"}, restoreUID)
		Expect(handler.ProcessRestore(ctx)).To(Succeed())

		Expect(listIPAddresses(cl)).To(HaveLen(1))
		ipa := getIPAddress(cl)
		Expect(ipa.GetOwnerReferences()).To(BeEmpty())
		Expect(ipa.GetLabels()).NotTo(HaveKey(annotations.LabelVirtualMachineUID))
		Expect(ipa.GetAnnotations()).To(HaveKeyWithValue(annotations.AnnVMOPRestore, restoreUID))
		Expect(ipa.Spec.Static.IP).To(Equal(address))
	})

	It("creates a Static IPAddress with the snapshot address when none exists", func() {
		cl, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		handler := NewMainIPAddressHandler(cl, name, namespace, address, []string{"vm-uid"}, restoreUID)
		Expect(handler.ProcessRestore(ctx)).To(Succeed())

		ipa := getIPAddress(cl)
		Expect(ipa.GetAnnotations()).To(HaveKeyWithValue(annotations.AnnVMOPRestore, restoreUID))
		Expect(ipa.Spec.NetworkRef.Kind).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
		Expect(ipa.Spec.NetworkRef.Name).To(Equal(network.MainClusterNetworkName))
		Expect(ipa.Spec.Type).To(Equal(sdnapi.IPAddressTypeStatic))
		Expect(ipa.Spec.Static.IP).To(Equal(address))
	})

	It("is idempotent on a repeated ProcessRestore", func() {
		cl, err := testutil.NewFakeClientWithObjects(newMirrorOwnedByVM())
		Expect(err).NotTo(HaveOccurred())

		handler := NewMainIPAddressHandler(cl, name, namespace, address, []string{"vm-uid"}, restoreUID)
		Expect(handler.ProcessRestore(ctx)).To(Succeed())
		detached := getIPAddress(cl)

		Expect(handler.ProcessRestore(ctx)).To(Succeed())

		Expect(listIPAddresses(cl)).To(HaveLen(1))
		Expect(getIPAddress(cl)).To(Equal(detached))
	})

	It("fails validation when the name is taken by an IPAddress with a different address", func() {
		mirror := newMirrorOwnedByVM()
		mirror.Spec.Static.IP = "10.66.10.99"
		cl, err := testutil.NewFakeClientWithObjects(mirror)
		Expect(err).NotTo(HaveOccurred())

		handler := NewMainIPAddressHandler(cl, name, namespace, address, []string{"vm-uid"}, restoreUID)
		Expect(handler.ValidateRestore(ctx)).To(HaveOccurred())
	})

	It("fails validation when the IPAddress belongs to another VM", func() {
		cl, err := testutil.NewFakeClientWithObjects(newMirrorOwnedByVM())
		Expect(err).NotTo(HaveOccurred())

		handler := NewMainIPAddressHandler(cl, name, namespace, address, []string{"restored-vm-uid"}, restoreUID)
		Expect(handler.ValidateRestore(ctx)).To(MatchError(ContainSubstring("in use by another virtual machine")))
		Expect(handler.ProcessRestore(ctx)).To(HaveOccurred())
		Expect(getIPAddress(cl).GetOwnerReferences()).NotTo(BeEmpty(), "the IPAddress of another VM must stay with it")
	})
})

func newAutoMainIPAddress(name, namespace, address string) *sdnapi.IPAddress {
	return &sdnapi.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: sdnapi.IPAddressSpec{
			NetworkRef: sdnapi.NetworkRef{
				Kind: v1alpha2.NetworksTypeClusterNetwork,
				Name: network.MainClusterNetworkName,
			},
			Type: sdnapi.IPAddressTypeAuto,
		},
		Status: sdnapi.IPAddressStatus{Address: address},
	}
}

var _ = Describe("MainIPAddressHandler for a VM on an Auto IPAddress", func() {
	const (
		ns         = "vms"
		ipName     = "vm-auto-ip"
		address    = "10.66.10.42"
		restoreUID = "restore-uid"
	)

	It("accepts an Auto IPAddress taking the snapshot address and pins it as Static", func() {
		ctx := context.Background()
		autoIPA := newAutoMainIPAddress(ipName, ns, address)
		cl, err := testutil.NewFakeClientWithObjects(autoIPA)
		Expect(err).NotTo(HaveOccurred())

		handler := NewMainIPAddressHandler(cl, ipName, ns, address, nil, restoreUID)
		Expect(handler.ValidateRestore(ctx)).To(Succeed(), "an Auto IPAddress has the address in the status, not in spec.static.ip")
		Expect(handler.ProcessRestore(ctx)).To(Succeed())

		ipa := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: ipName, Namespace: ns}, ipa)).To(Succeed())
		Expect(ipa.Spec.Type).To(Equal(sdnapi.IPAddressTypeStatic))
		Expect(ipa.Spec.Static.IP).To(Equal(address), "the address must be pinned so the restored VM keeps it")
		Expect(ipa.GetOwnerReferences()).To(BeEmpty())
	})

	It("rejects the restore when an Auto IPAddress of another name takes the address", func() {
		ctx := context.Background()
		ipaTakingAddress := newAutoMainIPAddress("other-ip", ns, address)
		cl, err := testutil.NewFakeClientWithObjects(ipaTakingAddress)
		Expect(err).NotTo(HaveOccurred())

		Expect(NewMainIPAddressHandler(cl, ipName, ns, address, nil, restoreUID).ValidateRestore(ctx)).
			To(MatchError(ContainSubstring(`in use by the IPAddress "other-ip"`)))
	})

	It("does not count a refused Static IPAddress as taking the address", func() {
		ctx := context.Background()
		refused := &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "refused-ip", Namespace: ns},
			Spec:       network.MainStaticIPAddressSpec(address),
			Status: sdnapi.IPAddressStatus{Conditions: []metav1.Condition{{
				Type:   sdnapi.ConditionAllocated,
				Status: metav1.ConditionFalse,
				Reason: sdnapi.ReasonIPAddressIsUsed,
			}}},
		}
		cl, err := testutil.NewFakeClientWithObjects(refused)
		Expect(err).NotTo(HaveOccurred())

		Expect(NewMainIPAddressHandler(cl, ipName, ns, address, nil, restoreUID).ValidateRestore(ctx)).To(Succeed())
	})

	It("sets ignoreLeaseTTLOnAllocation on the recreated IPAddress", func() {
		ctx := context.Background()
		cl, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		Expect(NewMainIPAddressHandler(cl, ipName, ns, address, nil, restoreUID).ProcessRestore(ctx)).To(Succeed())

		ipa := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: ipName, Namespace: ns}, ipa)).To(Succeed())
		Expect(ipa.Spec.Static.IgnoreLeaseTTLOnAllocation).To(BeTrue(), "the orphaned lease of the deleted VM still has this very address")
	})
})
