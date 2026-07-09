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

package network

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newNetworkObj(kind, name, namespace string, withPool bool) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	switch kind {
	case "ClusterNetwork":
		u.SetGroupVersionKind(ClusterNetworkGVK)
	case "Network":
		u.SetGroupVersionKind(NetworkGVK)
	}
	u.SetName(name)
	if namespace != "" {
		u.SetNamespace(namespace)
	}
	if withPool {
		Expect(unstructured.SetNestedField(u.Object, map[string]any{
			"ipAddressPoolRef": map[string]any{
				"kind": "ClusterIPAddressPool",
				"name": "demo-pool",
			},
		}, "spec", "ipam")).To(Succeed())
	}
	return u
}

func newIPAMFakeClient(objs ...client.Object) client.Client {
	scheme := apiruntime.NewScheme()
	_ = v1alpha2.AddToScheme(scheme)
	scheme.AddKnownTypeWithName(ClusterNetworkGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(NetworkGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(IPAddressGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: IPAddressGVK.Group, Version: IPAddressGVK.Version, Kind: "IPAddressList"}, &unstructured.UnstructuredList{})
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

// newSDNIPAddressObj creates an SDN IPAddress unstructured with the given name,
// namespace, VM UID label, networkRef, and allocated status.
func newSDNIPAddressObj(name, vmUID, networkKind, networkName, address string, allocated bool) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(IPAddressGVK)
	u.SetName(name)
	u.SetNamespace("ns")
	u.SetLabels(map[string]string{"virtualization.deckhouse.io/virtual-machine-uid": vmUID})
	_ = unstructured.SetNestedField(u.Object, map[string]any{"kind": networkKind, "name": networkName}, "spec", "networkRef")
	_ = unstructured.SetNestedField(u.Object, "Auto", "spec", "type")
	if address != "" {
		_ = unstructured.SetNestedField(u.Object, address, "status", "address")
	}
	status := "False"
	if allocated {
		status = "True"
		_ = unstructured.SetNestedField(u.Object, SDNIPAddressPhaseAllocated, "status", "phase")
	} else {
		_ = unstructured.SetNestedField(u.Object, SDNIPAddressPhasePending, "status", "phase")
	}
	_ = unstructured.SetNestedSlice(u.Object, []any{
		map[string]any{"type": "Allocated", "status": status, "reason": "IPAddressAllocated"},
	}, "status", "conditions")
	return u
}

var _ = Describe("EnrichWithIPAM", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("sets ipAssignmentMode=DHCP for additional networks with a pool (auto)", func() {
		ipAddr := newSDNIPAddressObj("vm-01-corp-auto", "vm-uid", "ClusterNetwork", "corp-net", "192.168.200.4", true)
		cl := newIPAMFakeClient(newNetworkObj("ClusterNetwork", "corp-net", "", true), ipAddr)
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{UID: "vm-uid"}, Spec: v1alpha2.VirtualMachineSpec{
			Networks: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain},
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"},
			},
		}}
		specs := InterfaceSpecList{
			{ID: 1, Type: v1alpha2.NetworksTypeMain, InterfaceName: NameDefaultInterface},
			{ID: 2, Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net", InterfaceName: "veth_cn1"},
		}

		out, err := EnrichWithIPAM(ctx, cl, "ns", vm, specs)

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(HaveLen(2))
		Expect(out[0].IPAssignmentMode).To(BeEmpty(), "Main is never enriched")
		Expect(out[1].IPAssignmentMode).To(Equal(IPAssignmentModeDHCP))
		Expect(out[1].IPAddressNames).To(Equal([]string{"vm-01-corp-auto"}))
	})

	It("does not set ipAssignmentMode for networks without a pool", func() {
		cl := newIPAMFakeClient(newNetworkObj("ClusterNetwork", "corp-net", "", false))
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{
			Networks: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"},
			},
		}}
		specs := InterfaceSpecList{
			{ID: 2, Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net", InterfaceName: "veth_cn1"},
		}

		out, err := EnrichWithIPAM(ctx, cl, "ns", vm, specs)

		Expect(err).NotTo(HaveOccurred())
		Expect(out[0].IPAssignmentMode).To(BeEmpty())
	})

	It("preserves ipAddressNames for static mode when IPAddress exists", func() {
		ipAddr := newSDNIPAddressObj("my-ip", "", "Network", "user-net", "192.168.201.10", true)
		cl := newIPAMFakeClient(newNetworkObj("Network", "user-net", "ns", true), ipAddr)
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{UID: "vm-uid"}, Spec: v1alpha2.VirtualMachineSpec{
			Networks: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "my-ip"},
			},
		}}
		specs := InterfaceSpecList{
			{ID: 2, Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", InterfaceName: "veth_n1", IPAddressNames: []string{"my-ip"}},
		}

		out, err := EnrichWithIPAM(ctx, cl, "ns", vm, specs)

		Expect(err).NotTo(HaveOccurred())
		Expect(out[0].IPAssignmentMode).To(Equal(IPAssignmentModeDHCP))
		Expect(out[0].IPAddressNames).To(Equal([]string{"my-ip"}))
	})
})

var _ = Describe("HasIPAM", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns true for ClusterNetwork with a pool", func() {
		cl := newIPAMFakeClient(newNetworkObj("ClusterNetwork", "corp-net", "", true))

		has, err := HasIPAM(ctx, cl, "ns", v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"})

		Expect(err).NotTo(HaveOccurred())
		Expect(has).To(BeTrue())
	})

	It("returns false for ClusterNetwork without a pool", func() {
		cl := newIPAMFakeClient(newNetworkObj("ClusterNetwork", "corp-net", "", false))

		has, err := HasIPAM(ctx, cl, "ns", v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"})

		Expect(err).NotTo(HaveOccurred())
		Expect(has).To(BeFalse())
	})

	It("returns true for namespaced Network with a pool", func() {
		cl := newIPAMFakeClient(newNetworkObj("Network", "user-net", "ns", true))

		has, err := HasIPAM(ctx, cl, "ns", v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net"})

		Expect(err).NotTo(HaveOccurred())
		Expect(has).To(BeTrue())
	})

	It("returns false when the network does not exist", func() {
		cl := newIPAMFakeClient()

		has, err := HasIPAM(ctx, cl, "ns", v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "missing"})

		Expect(err).NotTo(HaveOccurred())
		Expect(has).To(BeFalse())
	})

	It("returns false for the Main network", func() {
		cl := newIPAMFakeClient()

		has, err := HasIPAM(ctx, cl, "ns", v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain})

		Expect(err).NotTo(HaveOccurred())
		Expect(has).To(BeFalse())
	})

	It("returns false for a pool ref with an empty name", func() {
		u := newNetworkObj("ClusterNetwork", "corp-net", "", false)
		Expect(unstructured.SetNestedField(u.Object, map[string]any{
			"ipAddressPoolRef": map[string]any{
				"kind": "ClusterIPAddressPool",
				"name": "",
			},
		}, "spec", "ipam")).To(Succeed())
		cl := newIPAMFakeClient(u)

		has, err := HasIPAM(ctx, cl, "ns", v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"})

		Expect(err).NotTo(HaveOccurred())
		Expect(has).To(BeFalse())
	})
})

var _ = Describe("WillProvisionInterface", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns true for L2-only network without ipAddressName", func() {
		cl := newIPAMFakeClient(newNetworkObj("ClusterNetwork", "corp-net", "", false))
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{UID: "vm-uid"}}

		ok, err := WillProvisionInterface(ctx, cl, "ns", vm, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"})

		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
	})

	It("returns false for network without pool when ipAddressName is set (config error)", func() {
		cl := newIPAMFakeClient(newNetworkObj("ClusterNetwork", "corp-net", "", false))
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{UID: "vm-uid"}}

		ok, err := WillProvisionInterface(ctx, cl, "ns", vm, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net", IPAddressName: "my-ip"})

		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("returns true for static mode when IPAddress exists", func() {
		ipAddr := newSDNIPAddressObj("my-ip", "", "Network", "user-net", "192.168.201.10", true)
		cl := newIPAMFakeClient(newNetworkObj("Network", "user-net", "ns", true), ipAddr)
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{UID: "vm-uid"}}

		ok, err := WillProvisionInterface(ctx, cl, "ns", vm, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "my-ip"})

		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
	})

	It("returns false for static mode when IPAddress does not exist", func() {
		cl := newIPAMFakeClient(newNetworkObj("Network", "user-net", "ns", true))
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{UID: "vm-uid"}}

		ok, err := WillProvisionInterface(ctx, cl, "ns", vm, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "missing"})

		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})

	It("returns true for auto mode when IPAddress is allocated", func() {
		ipAddr := newSDNIPAddressObj("vm-auto", "vm-uid", "ClusterNetwork", "corp-net", "192.168.200.2", true)
		cl := newIPAMFakeClient(newNetworkObj("ClusterNetwork", "corp-net", "", true), ipAddr)
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{UID: "vm-uid", Name: "vm"}}

		ok, err := WillProvisionInterface(ctx, cl, "ns", vm, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"})

		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
	})

	It("returns false for auto mode when IPAddress is not allocated", func() {
		ipAddr := newSDNIPAddressObj("vm-auto", "vm-uid", "ClusterNetwork", "corp-net", "", false)
		cl := newIPAMFakeClient(newNetworkObj("ClusterNetwork", "corp-net", "", true), ipAddr)
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{UID: "vm-uid", Name: "vm"}}

		ok, err := WillProvisionInterface(ctx, cl, "ns", vm, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"})

		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
	})
})
