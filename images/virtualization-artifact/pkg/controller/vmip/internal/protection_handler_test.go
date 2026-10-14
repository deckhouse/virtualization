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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

var _ = Describe("ProtectionHandler mirror cleanup", func() {
	const (
		name      = "user-vmip"
		namespace = "ns"
		address   = "10.66.10.14"
		vmipUID   = types.UID("user-vmip-uid")
	)

	var (
		ctx  context.Context
		vmip *v1alpha2.VirtualMachineIPAddress
	)

	BeforeEach(func() {
		ctx = context.Background()
		now := metav1.Now()
		vmip = &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				UID:               vmipUID,
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerIPAddressCleanup},
			},
			Status: v1alpha2.VirtualMachineIPAddressStatus{
				Address: address,
				Conditions: []metav1.Condition{{
					Type:   vmipcondition.AttachedType.String(),
					Status: metav1.ConditionFalse,
					Reason: vmipcondition.VirtualMachineNotFound.String(),
				}},
			},
		}
	})

	// newMirror creates the Static SDN IPAddress the controller keeps for the vmip.
	newMirror := func(labeled bool) *sdnapi.IPAddress {
		mirror := &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: map[string]string{annotations.AnnMirrorOfVMIP: string(vmipUID)},
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
		if labeled {
			mirror.SetLabels(map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"})
		}
		return mirror
	}

	mirrorExists := func(cl client.Client) bool {
		err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &sdnapi.IPAddress{})
		if k8serrors.IsNotFound(err) {
			return false
		}
		Expect(err).NotTo(HaveOccurred())
		return true
	}

	handle := func(cl client.Client) {
		h := NewProtectionHandler(cl)
		_, err := h.Handle(ctx, vmip)
		Expect(err).NotTo(HaveOccurred())
		Expect(vmip.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerIPAddressCleanup))
	}

	It("keeps a foreign IPAddress that merely carries the vmip name", func() {
		foreign := newMirror(false)
		foreign.SetAnnotations(nil)
		cl, err := testutil.NewFakeClientWithObjects(foreign)
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		Expect(mirrorExists(cl)).To(BeTrue(), "a user resource must never be deleted by the vmip flow")
	})

	It("keeps an IPAddress carrying a hand-copied mirror annotation", func() {
		copied := newMirror(false)
		copied.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: "user-vmip"})
		cl, err := testutil.NewFakeClientWithObjects(copied)
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		Expect(mirrorExists(cl)).To(BeTrue(), "the marker names the uid of the vmip, so a copied value marks nothing")
	})

	It("deletes the unowned mirror of the vmip being deleted", func() {
		cl, err := testutil.NewFakeClientWithObjects(newMirror(false))
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		Expect(mirrorExists(cl)).To(BeFalse())
	})

	It("deletes the Auto mirror of a vmip deleted before the sdn module gave it an address", func() {
		pending := newMirror(false)
		pending.Spec.Type = sdnapi.IPAddressTypeAuto
		pending.Spec.Static = nil
		vmip.Status.Address = ""
		cl, err := testutil.NewFakeClientWithObjects(pending)
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		Expect(mirrorExists(cl)).To(BeFalse())
	})

	It("keeps the mirror labeled for a migrated VM", func() {
		cl, err := testutil.NewFakeClientWithObjects(newMirror(true))
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		Expect(mirrorExists(cl)).To(BeTrue())
	})

	It("keeps the mirror referenced by a VM spec (manual Legacy -> SDN transition)", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: namespace},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: name}},
			},
		}
		cl, err := testutil.NewFakeClientWithObjects(newMirror(false), vm)
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		Expect(mirrorExists(cl)).To(BeTrue())
	})

	It("takes the ownerRef on the vmip off the mirror a VM spec references", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: namespace},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: name}},
			},
		}
		mirror := newMirror(false)
		mirror.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(vmip, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineIPAddressKind))})
		cl, err := testutil.NewFakeClientWithObjects(mirror, vm)
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		got := &sdnapi.IPAddress{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, got)).To(Succeed())
		Expect(got.GetOwnerReferences()).To(BeEmpty(), "GC would delete the mirror together with the vmip")
	})

	It("keeps the mirror a pod still runs on", func() {
		mirror := newMirror(false)
		mirror.Status.UsedByPods = []string{"ns/d8v-vm-vm"}
		cl, err := testutil.NewFakeClientWithObjects(mirror)
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		Expect(mirrorExists(cl)).To(BeTrue())
	})

	It("deletes the mirror a machine left with its ownerRef once no pod runs on it", func() {
		// The machine moved to another vmip: the label is gone, the ownerRef stayed
		// while the former pod ran on the address.
		mirror := newMirror(false)
		mirror.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineKind,
			Name:       "vm",
			UID:        "vm-uid",
		}})
		cl, err := testutil.NewFakeClientWithObjects(mirror)
		Expect(err).NotTo(HaveOccurred())

		handle(cl)

		Expect(mirrorExists(cl)).To(BeFalse())
	})
})

var _ = Describe("Deleting vmip the VM has left", func() {
	const (
		namespace  = "ns"
		oldName    = "vmip-old"
		newName    = "vmip-new"
		oldAddress = "10.10.10.10"
		newAddress = "10.10.10.20"
		vmUID      = types.UID("vm-uid")
	)

	BeforeEach(testutil.EnableSDNFeatureGate)

	newAllocatedMirror := func(name, address string, vmipUID types.UID) *sdnapi.IPAddress {
		return &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: map[string]string{annotations.AnnMirrorOfVMIP: string(vmipUID)},
			},
			Spec: sdnapi.IPAddressSpec{
				NetworkRef: sdnapi.NetworkRef{Kind: v1alpha2.NetworksTypeClusterNetwork, Name: network.MainClusterNetworkName},
				Type:       sdnapi.IPAddressTypeStatic,
				Static:     &sdnapi.StaticIPAddress{IP: address},
			},
			Status: sdnapi.IPAddressStatus{
				Address:    address,
				Conditions: []metav1.Condition{{Type: sdnapi.ConditionAllocated, Status: metav1.ConditionTrue, Reason: "IPAddressAllocated"}},
			},
		}
	}

	It("deletes the released mirror despite Attached left from the previous reconcile", func() {
		ctx := context.Background()
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: namespace, UID: vmUID},
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineIPAddress: newName,
				Networks:                []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}},
			},
			Status: v1alpha2.VirtualMachineStatus{VirtualMachineIPAddress: newName, IPAddress: newAddress},
		}
		now := metav1.Now()
		oldVMIP := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:              oldName,
				Namespace:         namespace,
				UID:               "old-uid",
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerIPAddressCleanup},
			},
			Spec: v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
			Status: v1alpha2.VirtualMachineIPAddressStatus{
				Address:        oldAddress,
				VirtualMachine: vm.Name,
				Conditions: []metav1.Condition{
					{Type: vmipcondition.AttachedType.String(), Status: metav1.ConditionTrue, Reason: vmipcondition.Attached.String()},
					{Type: vmipcondition.BoundType.String(), Status: metav1.ConditionTrue, Reason: vmipcondition.Bound.String()},
				},
			},
		}
		// The VM has released the mirror of its previous vmip: no label, no ownerRef.
		oldMirror := newAllocatedMirror(oldName, oldAddress, oldVMIP.UID)
		newMirror := newAllocatedMirror(newName, newAddress, "new-uid")
		newMirror.SetLabels(map[string]string{annotations.LabelVirtualMachineUID: string(vmUID)})
		newMirror.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))})
		mainNetwork := &sdnapi.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{Name: network.MainClusterNetworkName},
			Spec: sdnapi.ClusterNetworkSpec{
				Type: sdnapi.ClusterNetworkTypePrimaryCNI,
				IPAM: &sdnapi.IPAMSpec{IPAddressPoolRef: &sdnapi.IPAddressPoolRef{Kind: sdnapi.ClusterIPAddressPoolKind, Name: "demo-pool"}},
			},
		}
		cl, err := testutil.NewFakeClientWithObjects(vm, mainNetwork, oldMirror, newMirror)
		Expect(err).NotTo(HaveOccurred())

		// The handlers in the controller order: BoundHandler runs MirrorStep before AttachedHandler.
		_, err = step.NewMirrorStep(nil, cl, conditions.NewConditionBuilder(vmipcondition.BoundType), recorder).Take(ctx, oldVMIP)
		Expect(err).NotTo(HaveOccurred())
		_, err = NewAttachedHandler(recorder, cl).Handle(ctx, oldVMIP)
		Expect(err).NotTo(HaveOccurred())
		_, err = NewProtectionHandler(cl).Handle(ctx, oldVMIP)
		Expect(err).NotTo(HaveOccurred())

		Expect(oldVMIP.Finalizers).NotTo(ContainElement(v1alpha2.FinalizerIPAddressCleanup))
		err = cl.Get(ctx, types.NamespacedName{Name: oldName, Namespace: namespace}, &sdnapi.IPAddress{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "a mirror left after the vmip keeps the address in the sdn pool forever")
	})
})
