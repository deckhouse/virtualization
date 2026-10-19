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
	"encoding/json"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// newMainClusterNetworkObj creates the PrimaryCNI ClusterNetwork "main".
func newMainClusterNetworkObj(networkType string, withPool bool) *unstructured.Unstructured {
	u := newNetworkObj("ClusterNetwork", MainClusterNetworkName, "", withPool)
	Expect(unstructured.SetNestedField(u.Object, networkType, "spec", "type")).To(Succeed())
	return u
}

var _ = Describe("MainIPAMAvailable", func() {
	ctx := context.Background()

	It("is available when ClusterNetwork main is PrimaryCNI with a pool", func() {
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true))
		Expect(MainIPAMAvailable(ctx, cl)).To(BeTrue())
	})

	It("is not available without a pool", func() {
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", false))
		Expect(MainIPAMAvailable(ctx, cl)).To(BeFalse())
	})

	It("is not available when the network type is not PrimaryCNI", func() {
		cl := newIPAMFakeClient(newMainClusterNetworkObj("Bridge", true))
		Expect(MainIPAMAvailable(ctx, cl)).To(BeFalse())
	})

	It("is not available when ClusterNetwork main does not exist", func() {
		cl := newIPAMFakeClient()
		Expect(MainIPAMAvailable(ctx, cl)).To(BeFalse())
	})

	It("is not available without the sdn module", func() {
		gate := featuregates.Default().(featuregate.MutableFeatureGate)
		Expect(gate.Set("SDN=false")).To(Succeed())
		DeferCleanup(func() { Expect(gate.Set("SDN=true")).To(Succeed()) })

		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true))
		Expect(MainIPAMAvailable(ctx, cl)).To(BeFalse())
	})
})

var _ = Describe("MainInterfaceSpecEnabled", func() {
	ctx := context.Background()
	kvvmWithMainInterfaceSpec := &virtv1.VirtualMachine{
		Spec: virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				annotations.AnnNetworksSpec: `[{"type":"Main","ipAddressNames":["vm-main"],"ipAddress":"10.66.10.14"}]`,
			}},
		}},
	}

	It("is required for a machine whose address is held by an IPAddress", func() {
		// Such a machine has nowhere else to take the address from.
		Expect(MainInterfaceSpecRequired(false, nil)).To(BeTrue())
		enabled, err := MainInterfaceSpecEnabled(ctx, newIPAMFakeClient(), false, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
	})

	It("is enabled, but not required, for a vmip-backed machine while the sdn module serves the Main network", func() {
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true))
		Expect(MainInterfaceSpecRequired(true, nil)).To(BeFalse())
		enabled, err := MainInterfaceSpecEnabled(ctx, cl, true, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
	})

	It("is not enabled for a vmip-backed machine in a cluster without the address pool", func() {
		enabled, err := MainInterfaceSpecEnabled(ctx, newIPAMFakeClient(), true, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeFalse())
	})

	It("stays required once the template has it, even after the pool is gone", func() {
		// The sdn module is what programs the route to such a machine, and
		// vm-route-forge that used to do it is gone: dropping the spec would leave
		// the machine with an address and no route to it.
		Expect(MainInterfaceSpecRequired(true, kvvmWithMainInterfaceSpec)).To(BeTrue())
		enabled, err := MainInterfaceSpecEnabled(ctx, newIPAMFakeClient(), true, kvvmWithMainInterfaceSpec)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
	})
})

var _ = Describe("UsesVMIP", func() {
	ctx := context.Background()

	newVM := func(vmipName string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: "vm-uid"},
			Spec:       v1alpha2.VirtualMachineSpec{VirtualMachineIPAddress: vmipName},
		}
	}

	It("gives the address of a pinned machine to the IPAddress", func() {
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true))
		pinned := newVM("")
		pinned.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"}}
		Expect(UsesVMIP(ctx, cl, pinned)).To(BeFalse())
	})

	It("ignores ipAddressName of the Main network where the sdn module does not serve it", func() {
		cl := newIPAMFakeClient()
		pinned := newVM("")
		pinned.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"}}
		Expect(UsesVMIP(ctx, cl, pinned)).To(BeTrue())
	})

	It("keeps a machine whose template has the Main interface spec on its IPAddress without the sdn module", func() {
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					annotations.AnnNetworksSpec: `[{"type":"Main","ipAddressNames":["my-static-ip"],"ipAddress":"10.66.10.14"}]`,
				}},
			}},
		}
		cl := newIPAMFakeClient(kvvm)
		pinned := newVM("")
		pinned.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"}}
		Expect(UsesVMIP(ctx, cl, pinned)).To(BeFalse())
	})

	It("leaves it with the vmip the owner named", func() {
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true))
		Expect(UsesVMIP(ctx, cl, newVM("my-vmip"))).To(BeTrue())
	})

	It("gives a new machine an IPAddress where the sdn module serves the Main network", func() {
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true))
		Expect(UsesVMIP(ctx, cl, newVM(""))).To(BeFalse())
	})

	It("keeps a machine on its IPAddress while that IPAddress is being deleted", func() {
		// Taken for a new machine, it would get a vmip with another address.
		terminating := newSDNIPAddressObj("vm-main", "vm-uid", "ClusterNetwork", MainClusterNetworkName, "10.66.10.14", true)
		now := metav1.Now()
		terminating.SetDeletionTimestamp(&now)
		terminating.SetFinalizers([]string{"network.deckhouse.io/ip-address-controller"})
		cl := newIPAMFakeClient(terminating)
		Expect(UsesVMIP(ctx, cl, newVM(""))).To(BeFalse())
	})

	It("gives a new machine a vmip while a vmip address is not allocated in the sdn module", func() {
		vmip := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "other-vmip", Namespace: "ns"},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: "10.66.10.5"},
		}
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true), vmip)
		Expect(UsesVMIP(ctx, cl, newVM(""))).To(BeTrue())
	})

	It("gives a new machine an IPAddress once every vmip address is allocated in the sdn module", func() {
		vmip := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "other-vmip", Namespace: "ns"},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: "10.66.10.5"},
		}
		mirror := newSDNIPAddressObj("other-vmip", "", "ClusterNetwork", MainClusterNetworkName, "10.66.10.5", true)
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true), vmip, mirror)
		Expect(UsesVMIP(ctx, cl, newVM(""))).To(BeFalse())
	})

	It("gives a new machine a vmip where it does not", func() {
		cl := newIPAMFakeClient()
		Expect(UsesVMIP(ctx, cl, newVM(""))).To(BeTrue())
	})

	It("keeps the vmip when both fields are set", func() {
		// The combination is rejected by the webhook; if it ever reaches here the
		// vmip wins, because that is the address the machine is running on.
		cl := newIPAMFakeClient(newMainClusterNetworkObj("PrimaryCNI", true))
		vm := newVM("my-vmip")
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"}}
		Expect(UsesVMIP(ctx, cl, vm)).To(BeTrue())
	})
})

var _ = Describe("ProvisionableInterfaceSpecs for the Main network", func() {
	ctx := context.Background()

	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: "vm-uid"}}
	}
	newVMNaming := func(ipAddressName string) *v1alpha2.VirtualMachine {
		vm := newVM()
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: ipAddressName}}
		return vm
	}
	mainSpecs := InterfaceSpecList{
		{ID: 1, Type: v1alpha2.NetworksTypeMain, InterfaceName: NameDefaultInterface},
	}

	It("puts the auto Main IPAddress and sets no ipAssignmentMode", func() {
		ipAddr := newSDNIPAddressObj("vm-abcde", "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		cl := newIPAMFakeClient(ipAddr)

		out, err := ProvisionableInterfaceSpecs(ctx, cl, newVM(), mainSpecs, true)

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(HaveLen(1))
		Expect(out[0].IPAddressNames).To(Equal([]string{"vm-abcde"}))
		// The address cni-cilium applies comes with the Main interface spec, not from a lookup.
		Expect(out[0].IPAddress).To(Equal("10.66.10.14"))
		Expect(out[0].IPAssignmentMode).To(BeEmpty())
	})

	It("keeps the Main interface spec unresolved while the IPAddress is not allocated", func() {
		ipAddr := newSDNIPAddressObj("vm-abcde", "vm-uid", "ClusterNetwork", "main", "", false)
		cl := newIPAMFakeClient(ipAddr)

		out, err := ProvisionableInterfaceSpecs(ctx, cl, newVM(), mainSpecs, true)

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(HaveLen(1))
		Expect(out[0].IPAddressNames).To(BeEmpty())
		Expect(out[0].IPAddress).To(BeEmpty())
	})

	It("validates a static Main IPAddress against the main ClusterNetwork", func() {
		ipAddr := newSDNIPAddressObj("my-static-ip", "", "ClusterNetwork", "main", "10.66.10.20", true)
		cl := newIPAMFakeClient(ipAddr)
		specs := InterfaceSpecList{
			{ID: 1, Type: v1alpha2.NetworksTypeMain, InterfaceName: NameDefaultInterface, IPAddressNames: []string{"my-static-ip"}},
		}

		out, err := ProvisionableInterfaceSpecs(ctx, cl, newVMNaming("my-static-ip"), specs, true)

		Expect(err).NotTo(HaveOccurred())
		Expect(out[0].IPAddressNames).To(Equal([]string{"my-static-ip"}))
		Expect(out[0].IPAddress).To(Equal("10.66.10.20"))
	})

	It("drops ipAddressNames of a static Main IPAddress bound to another network", func() {
		ipAddr := newSDNIPAddressObj("my-static-ip", "", "ClusterNetwork", "corp-net", "192.168.200.4", true)
		cl := newIPAMFakeClient(ipAddr)
		specs := InterfaceSpecList{
			{ID: 1, Type: v1alpha2.NetworksTypeMain, InterfaceName: NameDefaultInterface, IPAddressNames: []string{"my-static-ip"}},
		}

		out, err := ProvisionableInterfaceSpecs(ctx, cl, newVMNaming("my-static-ip"), specs, true)

		Expect(err).NotTo(HaveOccurred())
		Expect(out[0].IPAddressNames).To(BeEmpty())
	})

	It("passes the Main interface spec through untouched while it is not required", func() {
		cl := newIPAMFakeClient()

		out, err := ProvisionableInterfaceSpecs(ctx, cl, newVM(), mainSpecs, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(mainSpecs))
	})

	It("strips ipAddressNames from the Main interface spec while it is not required", func() {
		cl := newIPAMFakeClient()
		specs := InterfaceSpecList{
			{ID: 1, Type: v1alpha2.NetworksTypeMain, InterfaceName: NameDefaultInterface, IPAddressNames: []string{"my-static-ip"}},
		}

		out, err := ProvisionableInterfaceSpecs(ctx, cl, newVM(), specs, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(out[0].IPAddressNames).To(BeEmpty(), "a Main interface spec that is not enabled must never reach networks-spec")
	})
})

var _ = Describe("InterfaceSpecList.ForAnnotation", func() {
	It("drops the Main interface spec without ipAddressNames", func() {
		list := InterfaceSpecList{
			{ID: 1, Type: v1alpha2.NetworksTypeMain, InterfaceName: NameDefaultInterface},
			{ID: 2, Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net", InterfaceName: "veth_cn1"},
		}
		Expect(list.ForAnnotation()).To(HaveLen(1))
		Expect(list.ForAnnotation()[0].Type).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
	})

	// cni-cilium applies the address from the Main interface spec itself; a spec without one
	// leaves it with nothing to do, so it must not reach the annotation.
	It("drops the Main interface spec without an address", func() {
		list := InterfaceSpecList{
			{ID: 1, Type: v1alpha2.NetworksTypeMain, InterfaceName: NameDefaultInterface, IPAddressNames: []string{"vm-abcde"}},
		}
		Expect(list.ForAnnotation()).To(BeEmpty())
	})

	It("serializes the Main interface spec with the address cni-cilium applies", func() {
		list := InterfaceSpecList{
			{
				ID:             1,
				Type:           v1alpha2.NetworksTypeMain,
				InterfaceName:  NameDefaultInterface,
				IPAddressNames: []string{"vm-abcde"},
				IPAddress:      "10.66.10.14",
			},
		}

		str, err := list.ToString()
		Expect(err).NotTo(HaveOccurred())

		var entries []map[string]any
		Expect(json.Unmarshal([]byte(str), &entries)).To(Succeed())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0]).To(HaveKeyWithValue("type", "Main"))
		Expect(entries[0]).To(HaveKeyWithValue("ifName", MainPodInterfaceName))
		Expect(entries[0]).To(HaveKeyWithValue("ipAddressNames", ConsistOf("vm-abcde")))
		Expect(entries[0]).To(HaveKeyWithValue("ipAddress", "10.66.10.14"))
		Expect(entries[0]).NotTo(HaveKey("ipAssignmentMode"))
	})

	// The address of an additional network is applied by the sdn agent, which reads
	// it from the IPAddress the interface spec names.
	It("leaves the address out of the interface spec of an additional network", func() {
		list := InterfaceSpecList{
			{ID: 2, Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net", InterfaceName: "veth_cn1", IPAddressNames: []string{"vm-corp"}},
		}

		str, err := list.ToString()
		Expect(err).NotTo(HaveOccurred())

		var entries []map[string]any
		Expect(json.Unmarshal([]byte(str), &entries)).To(Succeed())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0]).NotTo(HaveKey("ipAddress"))
	})
})

var _ = Describe("EnsureSDNIPAddress for the Main network", func() {
	It("creates an IPAddress with networkRef ClusterNetwork/main", func() {
		ctx := context.Background()
		cl := newIPAMFakeClient()
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: "vm-uid"}}

		name, err := EnsureSDNIPAddress(ctx, cl, vm, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}, nil)
		Expect(err).NotTo(HaveOccurred())

		list := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, list)).To(Succeed())
		Expect(list.Items).To(HaveLen(1))
		Expect(list.Items[0].GetName()).To(Equal(name))
		Expect(list.Items[0].Spec.NetworkRef.Kind).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
		Expect(list.Items[0].Spec.NetworkRef.Name).To(Equal(MainClusterNetworkName))
	})
})

var _ = Describe("MainSDNIPAddressesByAddress", func() {
	ctx := context.Background()

	It("returns addresses of the main network only, static and allocated alike", func() {
		auto := newSDNIPAddressObj("main-auto", "", v1alpha2.NetworksTypeClusterNetwork, MainClusterNetworkName, "10.66.10.14", true)
		static := &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "main-static", Namespace: "ns"},
			Spec:       MainStaticIPAddressSpec("10.66.10.15"),
		}
		other := newSDNIPAddressObj("corp-auto", "", v1alpha2.NetworksTypeClusterNetwork, "corp-net", "192.168.200.4", true)
		cl := newIPAMFakeClient(auto, static, other)

		ips, err := MainSDNIPAddressesByAddress(ctx, cl)

		Expect(err).NotTo(HaveOccurred())
		Expect(ips).To(HaveLen(2))
		Expect(ips).To(HaveKey("10.66.10.14"))
		Expect(ips).To(HaveKey("10.66.10.15"))
	})
})

var _ = Describe("SDN IPAddress lifecycle around deletion", func() {
	ctx := context.Background()

	newVMWithUID := func(uid string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: types.UID(uid)},
		}
	}
	mainSpec := v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(MinID)}

	newTerminatingIPAddress := func(name, vmUID string) *sdnapi.IPAddress {
		u := newSDNIPAddressObj(name, vmUID, "ClusterNetwork", MainClusterNetworkName, "192.168.1.5", true)
		u.SetDeletionTimestamp(&metav1.Time{Time: metav1.Now().Time})
		u.SetFinalizers([]string{"network.deckhouse.io/ip-address-controller"})
		return u
	}

	It("does not create a replacement while the previous address is terminating", func() {
		vm := newVMWithUID("uid-1")
		cl := newIPAMFakeClient(newTerminatingIPAddress("vm-old", "uid-1"))

		items, err := ListSDNIPAddressesOfVM(ctx, cl, vm)
		Expect(err).NotTo(HaveOccurred())
		_, err = EnsureSDNIPAddress(ctx, cl, vm, mainSpec, items)
		Expect(err).To(MatchError(ErrIPAddressTerminating))

		// A generated name means every create succeeds, so a replacement here
		// would repeat on every reconcile.
		list := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, list)).To(Succeed())
		Expect(list.Items).To(HaveLen(1))
	})
})

var _ = Describe("SDN IPAddress access with the sdn module gone", func() {
	ctx := context.Background()

	// The apiserver answers a request for an unregistered kind with a NoMatch
	// error; that is what a disabled or removed sdn module looks like from here.
	noMatch := &meta.NoKindMatchError{GroupKind: sdnapi.SchemeGroupVersion.WithKind("IPAddress").GroupKind()}

	newClientWithoutSDN := func() client.Client {
		return fake.NewClientBuilder().
			WithScheme(newIPAMScheme()).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					if _, ok := obj.(*sdnapi.IPAddress); ok {
						return noMatch
					}
					return nil
				},
				List: func(_ context.Context, _ client.WithWatch, list client.ObjectList, _ ...client.ListOption) error {
					if _, ok := list.(*sdnapi.IPAddressList); ok {
						return noMatch
					}
					return nil
				},
			}).Build()
	}

	// A missing kind says only that the kind is missing; the callers decide what it means.
	It("passes the missing kind through", func() {
		_, err := GetSDNIPAddress(ctx, newClientWithoutSDN(), "ns", "vm-main")
		Expect(meta.IsNoMatchError(err)).To(BeTrue())

		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: types.UID("uid-1")}}
		_, err = ListSDNIPAddressesOfVM(ctx, newClientWithoutSDN(), vm)
		Expect(meta.IsNoMatchError(err)).To(BeTrue())
	})
})

var _ = Describe("Protecting the Main IPAddress of a running virtual machine", func() {
	ctx := context.Background()

	newRunningVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: types.UID("uid-1")},
			Status:     v1alpha2.VirtualMachineStatus{IPAddress: "10.66.10.14", Phase: v1alpha2.MachineRunning},
		}
	}

	ipAddressesOf := func(cl client.Client, vm *v1alpha2.VirtualMachine) []sdnapi.IPAddress {
		items, err := ListSDNIPAddressesOfVM(ctx, cl, vm)
		Expect(err).NotTo(HaveOccurred())
		return items
	}
	release := func(cl client.Client, vm *v1alpha2.VirtualMachine) error {
		return ReleaseTerminatingSDNIPAddresses(ctx, cl, vm, ipAddressesOf(cl, vm))
	}
	ensureMain := func(cl client.Client, vm *v1alpha2.VirtualMachine) (string, error) {
		return EnsureMainSDNIPAddress(ctx, cl, vm, ipAddressesOf(cl, vm))
	}

	getIPAddress := func(cl client.Client, name string) *sdnapi.IPAddress {
		ipa := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "ns"}, ipa)).To(Succeed())
		return ipa
	}

	It("recreates the lost resource as Static with the address the machine holds", func() {
		cl := newIPAMFakeClient()

		name, err := ensureMain(cl, newRunningVM())
		Expect(err).NotTo(HaveOccurred())

		ipa := getIPAddress(cl, name)
		Expect(ipa.Spec.Type).To(Equal(sdnapi.IPAddressTypeStatic))
		Expect(ipa.Spec.Static).NotTo(BeNil())
		Expect(ipa.Spec.Static.IP).To(Equal("10.66.10.14"), "a running machine must not get a different address")
		Expect(ipa.Spec.Static.IgnoreLeaseTTLOnAllocation).To(BeTrue(), "the lease of the deleted resource still holds this very address")
	})

	terminatingIPAddress := func(pods ...string) *sdnapi.IPAddress {
		ipa := newSDNIPAddressObj("vm-main", "uid-1", "ClusterNetwork", MainClusterNetworkName, "10.66.10.14", true)
		ipa.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "virtualization.deckhouse.io/v1alpha2", Kind: "VirtualMachine", Name: "vm", UID: "uid-1"}})
		now := metav1.Now()
		ipa.SetDeletionTimestamp(&now)
		ipa.SetFinalizers([]string{"network.deckhouse.io/ip-address-controller"})
		ipa.Status.UsedByPods = pods
		return ipa
	}

	It("lets the sdn module finish deleting a resource no pod uses", func() {
		vm := newRunningVM()
		vm.Status.Phase = v1alpha2.MachineStopped
		cl := newIPAMFakeClient(terminatingIPAddress())

		Expect(release(cl, vm)).To(Succeed())

		ipa := getIPAddress(cl, "vm-main")
		Expect(ipa.GetOwnerReferences()).To(BeEmpty(), "the ownerRef on the machine keeps the resource forever")
		Expect(ipa.GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "uid-1"), "the machine waits for its resource to go away")
	})

	It("keeps a deleted resource a pod still runs on", func() {
		cl := newIPAMFakeClient(terminatingIPAddress("ns/d8v-vm-vm-abcde"))

		Expect(release(cl, newRunningVM())).To(Succeed())

		Expect(getIPAddress(cl, "vm-main").GetOwnerReferences()).NotTo(BeEmpty())
	})

	// sdn counts only a pod with the Main interface spec in usedByPods; a pod on
	// cni.cilium.io/ipAddress alone still holds the address.
	It("keeps a deleted resource the running machine holds the address of", func() {
		cl := newIPAMFakeClient(terminatingIPAddress())

		Expect(release(cl, newRunningVM())).To(Succeed())

		Expect(getIPAddress(cl, "vm-main").GetOwnerReferences()).NotTo(BeEmpty())
	})

	It("keeps a deleted resource a paused machine holds the address of", func() {
		vm := newRunningVM()
		vm.Status.Phase = v1alpha2.MachinePause
		cl := newIPAMFakeClient(terminatingIPAddress())

		Expect(release(cl, vm)).To(Succeed())

		Expect(getIPAddress(cl, "vm-main").GetOwnerReferences()).NotTo(BeEmpty(), "the pod of a paused machine still holds the address")
	})

	It("lets the machine use a deleted IPAddress a pod still runs on", func() {
		cl := newIPAMFakeClient(terminatingIPAddress("ns/d8v-vm-vm-abcde"))
		vm := newRunningVM()

		ipa, err := GetMainSDNIPAddress(ctx, cl, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(ipa.GetName()).To(Equal("vm-main"))
		Expect(CheckMainSDNIPAddress(ctx, cl, vm, ipa)).To(Succeed(), "a rebuilt template would lose the Main interface spec of the running pod")
	})

	It("does not let a machine that is not running use a deleted IPAddress", func() {
		vm := newRunningVM()
		vm.Status.Phase = v1alpha2.MachineStarting
		cl := newIPAMFakeClient(terminatingIPAddress("ns/d8v-vm-vm-abcde"))

		ipa, err := GetMainSDNIPAddress(ctx, cl, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(CheckMainSDNIPAddress(ctx, cl, vm, ipa)).To(MatchError(ErrIPAddressBeingDeleted), "a starting machine waits for the replacement")
	})

	It("releases a terminating IPAddress of an additional network too", func() {
		ipa := terminatingIPAddress()
		ipa.Spec.NetworkRef = sdnapi.NetworkRef{Kind: "ClusterNetwork", Name: "corp-net"}
		cl := newIPAMFakeClient(ipa)

		Expect(release(cl, newRunningVM())).To(Succeed())

		Expect(getIPAddress(cl, "vm-main").GetOwnerReferences()).To(BeEmpty())
	})

	It("does not let the machine use a deleted IPAddress no pod uses", func() {
		cl := newIPAMFakeClient(terminatingIPAddress())
		vm := newRunningVM()

		ipa, err := GetMainSDNIPAddress(ctx, cl, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(CheckMainSDNIPAddress(ctx, cl, vm, ipa)).To(MatchError(ErrIPAddressBeingDeleted), "a machine that starts again waits for the replacement")
	})

	It("asks for a new address when another IPAddress uses the address of the machine", func() {
		named := newSDNIPAddressObj("vm-static-ip", "", "ClusterNetwork", MainClusterNetworkName, "10.66.10.14", true)
		named.Spec.Type = sdnapi.IPAddressTypeStatic
		named.Spec.Static = &sdnapi.StaticIPAddress{IP: "10.66.10.14"}
		cl := newIPAMFakeClient(named)

		name, err := ensureMain(cl, newRunningVM())
		Expect(err).NotTo(HaveOccurred())

		Expect(getIPAddress(cl, name).Spec.Type).To(Equal(sdnapi.IPAddressTypeAuto), "a Static one on an address in use stays Pending")
	})

	It("asks for a new address for a machine without one", func() {
		vm := newRunningVM()
		vm.Status.IPAddress = ""
		cl := newIPAMFakeClient()

		name, err := ensureMain(cl, vm)
		Expect(err).NotTo(HaveOccurred())

		Expect(getIPAddress(cl, name).Spec.Type).To(Equal(sdnapi.IPAddressTypeAuto))
	})
})

var _ = Describe("PreferredSDNIPAddress", func() {
	vm := &v1alpha2.VirtualMachine{Status: v1alpha2.VirtualMachineStatus{IPAddress: "10.66.10.14", Phase: v1alpha2.MachineRunning}}

	newIPAddress := func(name, address string, age time.Duration) sdnapi.IPAddress {
		ipa := newSDNIPAddressObj(name, "uid-1", "ClusterNetwork", MainClusterNetworkName, address, true)
		ipa.CreationTimestamp = metav1.NewTime(time.Now().Add(-age))
		return *ipa
	}

	It("returns nil for no IPAddress", func() {
		Expect(PreferredSDNIPAddress(vm, nil)).To(BeNil())
	})

	It("takes the oldest one", func() {
		items := []sdnapi.IPAddress{newIPAddress("a", "10.66.10.20", time.Minute), newIPAddress("b", "10.66.10.21", time.Hour)}
		Expect(PreferredSDNIPAddress(vm, items).Name).To(Equal("b"))
	})

	It("takes the one the machine runs on before an older one", func() {
		items := []sdnapi.IPAddress{newIPAddress("a", "10.66.10.20", time.Hour), newIPAddress("b", "10.66.10.14", time.Minute)}
		Expect(PreferredSDNIPAddress(vm, items).Name).To(Equal("b"))
	})

	// Only the mirror of the current vmip carries the label of the machine, so a
	// labelled mirror is where the address of the machine comes from.
	It("takes the mirror of the vmip before the one the machine runs on", func() {
		mirror := newIPAddress("b", "10.66.10.20", time.Minute)
		mirror.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: "vmip-uid"})
		items := []sdnapi.IPAddress{newIPAddress("a", "10.66.10.14", time.Hour), mirror}
		Expect(PreferredSDNIPAddress(vm, items).Name).To(Equal("b"))
	})

	It("takes a live one before a terminating one", func() {
		terminating := newIPAddress("a", "10.66.10.14", time.Hour)
		now := metav1.Now()
		terminating.SetDeletionTimestamp(&now)
		items := []sdnapi.IPAddress{terminating, newIPAddress("b", "10.66.10.20", time.Minute)}
		Expect(PreferredSDNIPAddress(vm, items).Name).To(Equal("b"))
	})
})

var _ = Describe("An IPAddress named for the Main network of two virtual machines", func() {
	ctx := context.Background()
	const address = "10.66.10.20"

	newSharedIPAddress := func(ownerUID string, allocated bool) *sdnapi.IPAddress {
		ipa := newSDNIPAddressObj("shared-ip", ownerUID, "ClusterNetwork", MainClusterNetworkName, "", allocated)
		if allocated {
			ipa.Status.Address = address
		}
		return ipa
	}
	newVMNaming := func(name, ipAddressName, statusAddress string, phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", UID: types.UID(name + "-uid")},
			Status:     v1alpha2.VirtualMachineStatus{IPAddress: statusAddress, Phase: phase},
		}
		if ipAddressName != "" {
			vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: ipAddressName}}
		}
		return vm
	}
	check := func(cl client.Client, vm *v1alpha2.VirtualMachine) error {
		ipa, err := GetMainSDNIPAddress(ctx, cl, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(ipa).NotTo(BeNil())
		return CheckMainSDNIPAddress(ctx, cl, vm, ipa)
	}
	expectConflictWith := func(err error, vmName string) {
		GinkgoHelper()
		var conflict *IPAddressConflictError
		Expect(errors.As(err, &conflict)).To(BeTrue(), "got %v", err)
		Expect(conflict.VMName).To(Equal(vmName))
		Expect(IsIPAddressConfigurationError(err)).To(BeTrue())
	}

	It("stays with the machine that already has its address, however old the other one is", func() {
		holder := newVMNaming("a", "shared-ip", address, v1alpha2.MachineRunning)
		other := newVMNaming("b", "shared-ip", "", v1alpha2.MachinePending)
		other.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour))
		cl := newIPAMFakeClient(newSharedIPAddress("", true), holder, other)

		Expect(check(cl, holder)).To(Succeed())
		expectConflictWith(check(cl, other), "a")
	})

	It("stays with a stopped machine that has its address", func() {
		holder := newVMNaming("a", "shared-ip", address, v1alpha2.MachineStopped)
		other := newVMNaming("b", "shared-ip", "", v1alpha2.MachinePending)
		cl := newIPAMFakeClient(newSharedIPAddress("", true), holder, other)

		Expect(check(cl, holder)).To(Succeed())
		expectConflictWith(check(cl, other), "a")
	})

	It("blocks both machines while neither has the allocated address", func() {
		a := newVMNaming("a", "shared-ip", "", v1alpha2.MachinePending)
		b := newVMNaming("b", "shared-ip", "", v1alpha2.MachinePending)
		cl := newIPAMFakeClient(newSharedIPAddress("", true), a, b)

		expectConflictWith(check(cl, a), "b")
		expectConflictWith(check(cl, b), "a")
	})

	It("blocks both machines while the IPAddress has no address", func() {
		a := newVMNaming("a", "shared-ip", "", v1alpha2.MachinePending)
		b := newVMNaming("b", "shared-ip", "", v1alpha2.MachinePending)
		cl := newIPAMFakeClient(newSharedIPAddress("", false), a, b)

		expectConflictWith(check(cl, a), "b")
		expectConflictWith(check(cl, b), "a")
	})

	It("is not given to a machine naming the IPAddress another machine got by its label", func() {
		owner := newVMNaming("a", "", address, v1alpha2.MachineRunning)
		other := newVMNaming("b", "shared-ip", "", v1alpha2.MachinePending)
		cl := newIPAMFakeClient(newSharedIPAddress("a-uid", true), owner, other)

		expectConflictWith(check(cl, other), "a")
	})

	It("is given to a machine naming the IPAddress of a deleted machine", func() {
		vm := newVMNaming("b", "shared-ip", "", v1alpha2.MachinePending)
		cl := newIPAMFakeClient(newSharedIPAddress("gone-uid", true), vm)

		Expect(check(cl, vm)).To(Succeed(), "a stale label of a deleted owner is not a conflict")
	})
})
