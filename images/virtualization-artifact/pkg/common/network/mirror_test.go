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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newStaticIPAddressObj(name, address string) *sdnapi.IPAddress {
	ipa := newSDNIPAddressObj(name, "", v1alpha2.NetworksTypeClusterNetwork, MainClusterNetworkName, address, true)
	ipa.Spec.Type = sdnapi.IPAddressTypeStatic
	ipa.Spec.Static = &sdnapi.StaticIPAddress{IP: address}
	return ipa
}

// ensureVMIPAddressTaken passes the owner the way the vmip controller does.
func ensureVMIPAddressTaken(ctx context.Context, cl client.Client, vmip *v1alpha2.VirtualMachineIPAddress, vmMovedToIPAddress bool) (*sdnapi.IPAddress, error) {
	vm, err := GetVMOfVMIP(ctx, cl, vmip)
	Expect(err).NotTo(HaveOccurred())
	return EnsureVMIPAddressTaken(ctx, cl, vmip, MirrorOwnerRef(vmip, vm, vmMovedToIPAddress))
}

// newVMObj creates the VM "vm" the vmips of the tests are owned by.
func newVMObj() *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: "vm-uid"}}
}

var _ = Describe("EnsureVMIPAddressTaken", func() {
	const address = "10.66.10.14"

	ctx := context.Background()

	newManagedVMIP := func() *v1alpha2.VirtualMachineIPAddress {
		return &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm-abcde",
				Namespace: "ns",
				Labels:    map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.VirtualMachineKind,
					Name:       "vm",
					UID:        "vm-uid",
				}},
			},
			Status: v1alpha2.VirtualMachineIPAddressStatus{Address: address},
		}
	}

	getIPAddress := func(cl client.Client, name string) *sdnapi.IPAddress {
		ipa := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "ns"}, ipa)).To(Succeed())
		return ipa
	}

	It("creates a controller-owned mirror for a managed vmip", func() {
		cl := newIPAMFakeClient(newVMObj())

		ipa, err := ensureVMIPAddressTaken(ctx, cl, newManagedVMIP(), false)

		Expect(err).NotTo(HaveOccurred())
		Expect(ipa.GetName()).To(Equal(newManagedVMIP().Name), "a retry after a failed create must find the same mirror")
		mirror := getIPAddress(cl, ipa.GetName())
		Expect(mirror.GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
		Expect(mirror.GetOwnerReferences()).To(HaveLen(1))
		Expect(mirror.GetOwnerReferences()[0].Kind).To(Equal(v1alpha2.VirtualMachineKind))
		Expect(mirror.Spec.NetworkRef.Kind).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
		Expect(mirror.Spec.NetworkRef.Name).To(Equal(MainClusterNetworkName))
		Expect(mirror.Spec.Type).To(Equal(sdnapi.IPAddressTypeStatic))
		Expect(mirror.Spec.Static.IP).To(Equal(address))
	})

	newUserVMIP := func(vmName string) *v1alpha2.VirtualMachineIPAddress {
		return &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "user-vmip", Namespace: "ns"},
			Status: v1alpha2.VirtualMachineIPAddressStatus{
				Address:        address,
				VirtualMachine: vmName,
			},
		}
	}

	newUserVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "user-vm", Namespace: "ns", UID: "user-vm-uid"},
		}
	}

	It("creates an ownerless mirror named after a user-created vmip that no machine uses yet", func() {
		cl := newIPAMFakeClient()

		ipa, err := ensureVMIPAddressTaken(ctx, cl, newUserVMIP(""), false)

		Expect(err).NotTo(HaveOccurred())
		Expect(ipa.GetName()).To(Equal("user-vmip"))
		mirror := getIPAddress(cl, ipa.GetName())
		Expect(mirror.GetOwnerReferences()).To(BeEmpty())
		Expect(mirror.Spec.Static.IP).To(Equal(address))
	})

	// Without the owner the sdn module counts the address as unused: no routes
	// are programmed for it and the lease may be reclaimed while the machine runs.
	It("owns the mirror of a user-created vmip by the machine that uses it", func() {
		cl := newIPAMFakeClient(newUserVM())

		ipa, err := ensureVMIPAddressTaken(ctx, cl, newUserVMIP("user-vm"), false)

		Expect(err).NotTo(HaveOccurred())
		Expect(ipa.GetName()).To(Equal("user-vmip"))
		mirror := getIPAddress(cl, ipa.GetName())
		Expect(mirror.GetOwnerReferences()).To(HaveLen(1))
		Expect(mirror.GetOwnerReferences()[0].Kind).To(Equal(v1alpha2.VirtualMachineKind))
		Expect(mirror.GetOwnerReferences()[0].Name).To(Equal("user-vm"))
		// The address is the vmip's already: a lease left behind by a resource
		// that is gone must not keep the mirror from writing that down.
		Expect(mirror.Spec.Static.IgnoreLeaseTTLOnAllocation).To(BeTrue())
		// The pod of the machine gets the Main interface spec only for an address the VM
		// lookup finds, and without the entry the sdn module never learns the
		// node of the machine — its route goes away with vm-route-forge.
		Expect(mirror.GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, string(mirror.GetOwnerReferences()[0].UID)))
	})

	It("adopts an ownerless mirror once the machine takes the address", func() {
		vmip := newUserVMIP("user-vm")
		vmip.UID = "user-vmip-uid"
		ownerless := newStaticIPAddressObj("user-vmip", address)
		ownerless.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: string(vmip.UID)})
		cl := newIPAMFakeClient(newUserVM(), ownerless)

		ipa, err := ensureVMIPAddressTaken(ctx, cl, vmip, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(ipa.GetName()).To(Equal("user-vmip"))
		mirror := getIPAddress(cl, ipa.GetName())
		Expect(mirror.GetOwnerReferences()).To(HaveLen(1))
		Expect(mirror.GetOwnerReferences()[0].Name).To(Equal("user-vm"))
		Expect(mirror.GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, string(mirror.GetOwnerReferences()[0].UID)))
	})

	It("does not adopt an ownerless user IPAddress on the same address", func() {
		// Owned by the machine, the user resource would be deleted together with it.
		cl := newIPAMFakeClient(newUserVM(), newStaticIPAddressObj("user-ip", address))

		ipa, err := ensureVMIPAddressTaken(ctx, cl, newUserVMIP("user-vm"), false)

		Expect(err).NotTo(HaveOccurred())
		Expect(ipa.GetName()).To(Equal("user-ip"))
		userIPA := getIPAddress(cl, ipa.GetName())
		Expect(userIPA.GetOwnerReferences()).To(BeEmpty())
		Expect(userIPA.GetLabels()[annotations.LabelVirtualMachineUID]).To(BeEmpty())
	})

	It("keeps the mirror of the address owner when a vmip whose VM runs on an IPAddress of its own reconciles", func() {
		owned := newStaticIPAddressObj("owned-mirror", address)
		owned.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineKind,
			Name:       "other-vm",
			UID:        "other-uid",
		}})
		cl := newIPAMFakeClient(newUserVM(), owned)

		ipa, err := ensureVMIPAddressTaken(ctx, cl, newUserVMIP("user-vm"), true)

		Expect(err).NotTo(HaveOccurred())
		Expect(ipa.GetName()).To(Equal("owned-mirror"))
		Expect(getIPAddress(cl, ipa.GetName()).GetOwnerReferences()[0].Name).To(Equal("other-vm"))
	})

	It("is idempotent: an existing IPAddress with the same address is reused", func() {
		cl := newIPAMFakeClient(newStaticIPAddressObj("existing-mirror", address))

		ipa, err := ensureVMIPAddressTaken(ctx, cl, newManagedVMIP(), false)

		Expect(err).NotTo(HaveOccurred())
		Expect(ipa.GetName()).To(Equal("existing-mirror"))
		ipas := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, ipas)).To(Succeed())
		Expect(ipas.Items).To(HaveLen(1))
	})

	It("creates an unlabelled mirror owned by the vmip when its VM moved to an IPAddress", func() {
		vmip := newManagedVMIP()
		vmip.UID = "vmip-uid"
		cl := newIPAMFakeClient(newVMObj())

		ipa, err := ensureVMIPAddressTaken(ctx, cl, vmip, true)

		Expect(err).NotTo(HaveOccurred())
		mirror := getIPAddress(cl, ipa.GetName())
		Expect(mirror.GetLabels()).To(BeEmpty(), "a second labelled IPAddress would make the lookup of the IPAddress of the VM ambiguous")
		Expect(mirror.GetOwnerReferences()).To(HaveLen(1))
		Expect(mirror.GetOwnerReferences()[0].Kind).To(Equal(v1alpha2.VirtualMachineIPAddressKind))
	})

	It("waits while the IPAddress on the address is being deleted instead of creating a second one", func() {
		terminating := newSDNIPAddressObj("vm-old", "vm-uid", v1alpha2.NetworksTypeClusterNetwork, MainClusterNetworkName, "", false)
		terminating.Spec.Static = &sdnapi.StaticIPAddress{IP: address}
		terminating.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		terminating.SetFinalizers([]string{"network.deckhouse.io/ip-address-controller"})
		terminating.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: "vmip"})
		cl := newIPAMFakeClient(terminating, newVMObj())

		_, err := ensureVMIPAddressTaken(ctx, cl, newManagedVMIP(), false)
		Expect(err).To(MatchError(ErrIPAddressTerminating))

		// The sdn module would refuse a second IPAddress the address while the first lives.
		ipas := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, ipas)).To(Succeed())
		Expect(ipas.Items).To(HaveLen(1))
	})
})

var _ = Describe("GetIPAddressTakingVMIPAddress", func() {
	const address = "10.66.10.14"
	ctx := context.Background()

	vmip := &v1alpha2.VirtualMachineIPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "vmip", Namespace: "ns", UID: "vmip-uid"},
		Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: address},
	}

	newAutoIPAddress := func(name, mirrorOfVMIPUID string) *sdnapi.IPAddress {
		ipa := newSDNIPAddressObj(name, "", v1alpha2.NetworksTypeClusterNetwork, MainClusterNetworkName, address, true)
		ipa.Spec.Type = sdnapi.IPAddressTypeAuto
		if mirrorOfVMIPUID != "" {
			ipa.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: mirrorOfVMIPUID})
		}
		return ipa
	}

	find := func(objs ...client.Object) *sdnapi.IPAddress {
		ipa, err := GetIPAddressTakingVMIPAddress(ctx, newIPAMFakeClient(objs...), vmip)
		Expect(err).NotTo(HaveOccurred())
		return ipa
	}

	It("finds the Auto mirror by the address the sdn module gave it", func() {
		Expect(find(newAutoIPAddress("vmip", "vmip-uid")).GetName()).To(Equal("vmip"))
	})

	It("does not take the Auto IPAddress of a machine on the sdn module for a mirror", func() {
		Expect(find(newAutoIPAddress("vm-main", ""))).To(BeNil())
	})

	It("takes the IPAddress the sdn module allocated the address to, whatever the list order", func() {
		refused := newStaticIPAddressObj("a-user-ip", address)
		refused.Status.Conditions[0].Status = metav1.ConditionFalse
		refused.Status.Conditions[0].Reason = sdnapi.ReasonIPAddressIsUsed
		mirror := newStaticIPAddressObj("vmip", address)
		mirror.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: "vmip-uid"})

		Expect(find(refused, mirror).GetName()).To(Equal("vmip"))
	})

	It("takes a live IPAddress over one being deleted", func() {
		terminating := newStaticIPAddressObj("a-old", address)
		terminating.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
		terminating.SetFinalizers([]string{"network.deckhouse.io/ip-address-controller"})

		Expect(find(terminating, newStaticIPAddressObj("b-new", address)).GetName()).To(Equal("b-new"))
	})
})

var _ = Describe("EnsureAutoMirror", func() {
	ctx := context.Background()

	newVMIP := func() *v1alpha2.VirtualMachineIPAddress {
		return &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "vmip", Namespace: "ns", UID: "vmip-uid"},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
		}
	}

	It("creates an Auto mirror once and returns it afterwards", func() {
		cl := newIPAMFakeClient()

		created, err := EnsureAutoMirror(ctx, cl, newVMIP(), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Spec.Type).To(Equal(sdnapi.IPAddressTypeAuto))
		Expect(created.GetAnnotations()).To(HaveKeyWithValue(annotations.AnnMirrorOfVMIP, "vmip-uid"))

		again, err := EnsureAutoMirror(ctx, cl, newVMIP(), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(again.GetName()).To(Equal(created.GetName()))
		ipas := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, ipas)).To(Succeed())
		Expect(ipas.Items).To(HaveLen(1))
	})

	It("reports nil when the name is taken by a foreign IPAddress", func() {
		cl := newIPAMFakeClient(newStaticIPAddressObj("vmip", "10.66.10.99"))

		mirror, err := EnsureAutoMirror(ctx, cl, newVMIP(), nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(mirror).To(BeNil())
	})
})

var _ = Describe("ReleaseTerminatingMirror", func() {
	const address = "10.66.10.14"
	ctx := context.Background()

	vmip := &v1alpha2.VirtualMachineIPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "vm-abcde", Namespace: "ns", UID: "vmip-uid"},
		Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: address},
	}

	newTerminatingMirror := func(usedByPods []string) *sdnapi.IPAddress {
		terminating := newSDNIPAddressObj(vmip.Name, "vm-uid", v1alpha2.NetworksTypeClusterNetwork, MainClusterNetworkName, address, true)
		terminating.Spec.Static = &sdnapi.StaticIPAddress{IP: address}
		terminating.Status.UsedByPods = usedByPods
		terminating.SetDeletionTimestamp(&metav1.Time{Time: metav1.Now().Time})
		terminating.SetFinalizers([]string{"network.deckhouse.io/ip-address-controller"})
		terminating.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: string(vmip.UID)})
		terminating.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineKind,
			Name:       "vm",
			UID:        "vm-uid",
		}})
		return terminating
	}

	getMirror := func(cl client.Client) *sdnapi.IPAddress {
		mirror := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: vmip.Name, Namespace: "ns"}, mirror)).To(Succeed())
		return mirror
	}

	It("drops the VM label of a terminating mirror already without the ownerRef", func() {
		terminating := newTerminatingMirror(nil)
		terminating.SetOwnerReferences(nil)
		cl := newIPAMFakeClient(terminating)

		Expect(ReleaseTerminatingMirror(ctx, cl, vmip, nil)).To(Succeed())

		Expect(getMirror(cl).GetLabels()).NotTo(HaveKey(annotations.LabelVirtualMachineUID))
	})

	It("lets go of a deleted mirror nothing uses any more", func() {
		cl := newIPAMFakeClient(newTerminatingMirror(nil))

		Expect(ReleaseTerminatingMirror(ctx, cl, vmip, nil)).To(Succeed())

		released := getMirror(cl)
		Expect(released.GetOwnerReferences()).To(BeEmpty())
		Expect(released.GetLabels()).NotTo(HaveKey(annotations.LabelVirtualMachineUID))
	})

	It("keeps the reservation of a deleted mirror a pod still runs on", func() {
		// The reservation is what keeps the route of a machine whose address
		// resource is on its way out.
		cl := newIPAMFakeClient(newTerminatingMirror([]string{"ns/d8v-vm-vm"}))

		Expect(ReleaseTerminatingMirror(ctx, cl, vmip, nil)).To(Succeed())

		Expect(getMirror(cl).GetOwnerReferences()).To(HaveLen(1))
	})

	// sdn lists in usedByPods only a pod with the Main interface spec; a pod on
	// cni.cilium.io/ipAddress alone still uses the address.
	It("keeps the reservation of a deleted mirror the running machine uses the address of", func() {
		vm := newVMObj()
		vm.Status.Phase = v1alpha2.MachineRunning
		vm.Status.IPAddress = address
		cl := newIPAMFakeClient(newTerminatingMirror(nil))

		Expect(ReleaseTerminatingMirror(ctx, cl, vmip, vm)).To(Succeed())

		Expect(getMirror(cl).GetOwnerReferences()).To(HaveLen(1))
	})
})

var _ = Describe("ReleaseMirrorsOfPreviousVMIPs", func() {
	ctx := context.Background()

	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns", UID: "vm-uid"}}

	newLabelledMirror := func(name, address, vmipUID string) *sdnapi.IPAddress {
		ipa := newStaticIPAddressObj(name, address)
		ipa.SetLabels(map[string]string{annotations.LabelVirtualMachineUID: string(vm.UID)})
		ipa.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: vmipUID})
		ipa.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))})
		return ipa
	}

	It("leaves the label of the machine on the mirror of its current vmip only", func() {
		current := &v1alpha2.VirtualMachineIPAddress{ObjectMeta: metav1.ObjectMeta{Name: "vmip-new", Namespace: "ns", UID: "vmip-new-uid"}}
		cl := newIPAMFakeClient(
			newLabelledMirror("vmip-old", "10.66.10.5", "vmip-old-uid"),
			newLabelledMirror("vmip-new", "10.66.10.6", "vmip-new-uid"),
		)

		Expect(ReleaseMirrorsOfPreviousVMIPs(ctx, cl, vm, current)).To(Succeed())

		ipa, err := GetSDNIPAddressOfVM(ctx, cl, vm, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain})
		Expect(err).NotTo(HaveOccurred())
		Expect(ipa).NotTo(BeNil())
		Expect(ipa.GetName()).To(Equal("vmip-new"), "the Main interface spec of the next pod is built from this lookup")

		previous := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: "vmip-old", Namespace: "ns"}, previous)).To(Succeed())
		Expect(previous.GetOwnerReferences()).To(BeEmpty(), "the mirror goes back to its vmip and is removed with it")
	})

	It("keeps the ownerRef of the machine on a previous mirror its pod still runs on", func() {
		current := &v1alpha2.VirtualMachineIPAddress{ObjectMeta: metav1.ObjectMeta{Name: "vmip-new", Namespace: "ns", UID: "vmip-new-uid"}}
		inUse := newLabelledMirror("vmip-old", "10.66.10.5", "vmip-old-uid")
		inUse.Status.UsedByPods = []string{"ns/d8v-vm-vm"}
		cl := newIPAMFakeClient(inUse, newLabelledMirror("vmip-new", "10.66.10.6", "vmip-new-uid"))

		Expect(ReleaseMirrorsOfPreviousVMIPs(ctx, cl, vm, current)).To(Succeed())

		previous := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: "vmip-old", Namespace: "ns"}, previous)).To(Succeed())
		Expect(previous.GetLabels()).NotTo(HaveKey(annotations.LabelVirtualMachineUID))
		Expect(previous.GetOwnerReferences()).To(HaveLen(1), "without an owner the sdn module drops the route of the running pod")
	})

	It("drops the ownerRef of a released mirror once the pod has moved off it", func() {
		// Deleting the vmip of such a mirror would leave it terminating for as long
		// as the machine lives: the sdn module counts an owned address as used.
		current := &v1alpha2.VirtualMachineIPAddress{ObjectMeta: metav1.ObjectMeta{Name: "vmip-new", Namespace: "ns", UID: "vmip-new-uid"}}
		released := newLabelledMirror("vmip-old", "10.66.10.5", "vmip-old-uid")
		released.SetLabels(nil)
		cl := newIPAMFakeClient(released, newLabelledMirror("vmip-new", "10.66.10.6", "vmip-new-uid"))

		Expect(ReleaseMirrorsOfPreviousVMIPs(ctx, cl, vm, current)).To(Succeed())

		previous := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: "vmip-old", Namespace: "ns"}, previous)).To(Succeed())
		Expect(previous.GetOwnerReferences()).To(BeEmpty())
	})
})
