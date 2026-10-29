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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

type denyVMIPCreateClient struct{ client.Client }

func (c denyVMIPCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*v1alpha2.VirtualMachineIPAddress); ok {
		return errors.New(`admission webhook "vmip.validate" denied the request: the IP address is taken`)
	}
	return c.Client.Create(ctx, obj, opts...)
}

var _ = Describe("checkReceiverCapacity", func() {
	source := &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
		VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-root", Size: 1144762368}},
	}}
	here := func(capacity string) map[string]*v1alpha2.VirtualDisk {
		return map[string]*v1alpha2.VirtualDisk{"root": {Status: v1alpha2.VirtualDiskStatus{Capacity: capacity}}}
	}

	It("refuses a disk whose capacity is below the size of the source before the copy", func() {
		Expect(checkReceiverCapacity(source, here("1Gi"))).To(ContainSubstring(`"root"`))
	})
	It("accepts a disk that can hold the source", func() {
		Expect(checkReceiverCapacity(source, here("2Gi"))).To(BeEmpty())
		Expect(checkReceiverCapacity(source, here(""))).To(BeEmpty())
	})
})

var _ = Describe("checkReceiverDisks", func() {
	source := &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
		VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-root", Size: 1144762368}},
	}}
	here := func(size int64) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
			VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-root", Size: size}},
		}}
	}

	It("refuses a block device smaller than the one of the source", func() {
		Expect(checkReceiverDisks(source, here(1144717312))).To(ContainSubstring(`"vd-root"`))
	})
	It("accepts a block device not smaller than the source", func() {
		Expect(checkReceiverDisks(source, here(1144762368))).To(BeEmpty())
		Expect(checkReceiverDisks(source, here(1144762368+4096))).To(BeEmpty())
	})
	It("waits for the sizes of the receiving pod", func() {
		Expect(checkReceiverDisks(source, nil)).To(BeEmpty())
		Expect(checkReceiverDisks(source, here(0))).To(BeEmpty())
	})
})

var _ = Describe("isReceiving", func() {
	It("is true until the VM runs here, fails or the request goes", func() {
		r := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}
		Expect(isReceiving(r)).To(BeTrue())
		r.Status.Phase = intvirtv1alpha1.ReceiverReady
		Expect(isReceiving(r)).To(BeTrue())
		r.Status.Phase = intvirtv1alpha1.ReceiverRunning
		Expect(isReceiving(r)).To(BeFalse())
		r.Status.Phase = intvirtv1alpha1.ReceiverFailed
		Expect(isReceiving(r)).To(BeFalse())
		Expect(isReceiving(nil)).To(BeFalse())
	})
})

var _ = Describe("loadMigrationReceiver", func() {
	scheme := runtime.NewScheme()
	Expect(intvirtv1alpha1.AddToScheme(scheme)).To(Succeed())
	receiver := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Namespace: "app-x", Name: "web-01", UID: "uid-1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(receiver).Build()
	vm := func(mark string) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Namespace: "app-x", Name: "web-01"}}
		if mark != "" {
			vm.Annotations = map[string]string{annotations.AnnVMMigrationReceiver: mark}
		}
		return vm
	}

	It("returns the request the VM was created for", func() {
		r, err := loadMigrationReceiver(context.Background(), cl, vm("uid-1"))
		Expect(err).NotTo(HaveOccurred())
		Expect(r).NotTo(BeNil())
	})
	It("ignores a request of the same name for another VM", func() {
		for _, mark := range []string{"", "uid-2"} {
			r, err := loadMigrationReceiver(context.Background(), cl, vm(mark))
			Expect(err).NotTo(HaveOccurred())
			Expect(r).To(BeNil())
		}
	})
})

var _ = Describe("isAdmissionDenial", func() {
	It("hands KubeVirt the sizes of the source images", func() {
		kvvm := &virtv1.VirtualMachine{Spec: virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{}}}
		Expect(setSourceVolumeSizes(kvvm, &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
			VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-root", Size: 2028994560}, {Name: "cloudinit"}},
		}})).To(Succeed())
		Expect(kvvm.Spec.Template.ObjectMeta.Annotations).To(HaveKeyWithValue(annotations.AnnSourceVolumeSizes, `{"vd-root":2028994560}`))
	})

	It("tells a webhook denial from other errors", func() {
		Expect(isAdmissionDenial(errors.New(`admission webhook "vmip.validate" denied the request: the IP address is taken`))).To(BeTrue())
		Expect(isAdmissionDenial(errors.New("connection refused"))).To(BeFalse())
		Expect(isAdmissionDenial(k8serrors.NewForbidden(schema.GroupResource{Resource: "virtualmachineipaddresses"}, "web", errors.New("RBAC: access denied")))).To(BeFalse())
		Expect(isAdmissionDenial(nil)).To(BeFalse())
	})
})

var _ = Describe("receivingPodMessage", func() {
	It("tells why the receiving pod does not start", func() {
		kvvm := &virtv1.VirtualMachine{}
		Expect(receivingPodMessage(kvvm)).To(BeEmpty())
		kvvm.Status.Conditions = []virtv1.VirtualMachineCondition{{
			Type: virtv1.VirtualMachineConditionType(corev1.PodScheduled), Status: corev1.ConditionFalse,
			Reason: "Unschedulable", Message: "0/4 nodes are available: 2 node(s) didn't match Pod's node affinity/selector",
		}}
		Expect(receivingPodMessage(kvvm)).To(Equal("the receiving pod cannot be scheduled: Unschedulable: 0/4 nodes are available: 2 node(s) didn't match Pod's node affinity/selector"))
	})
})

var _ = Describe("syncMigrationReceiver", func() {
	It("finishes a receive whose request is gone when the migration has succeeded", func() {
		const ns = "app-x"
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web", Namespace: ns,
				Annotations: map[string]string{annotations.AnnVMMigrationReceiver: "receiver-uid"},
			},
			Spec: v1alpha2.VirtualMachineSpec{RunPolicy: v1alpha2.ManualPolicy},
		}
		received := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Name: "receiver-uid", Namespace: ns},
			Spec: virtv1.VirtualMachineInstanceMigrationSpec{
				VMIName: "web",
				Receive: &virtv1.VirtualMachineInstanceMigrationTarget{MigrationID: "receiver-uid"},
			},
			Status: virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
		}
		fakeClient, _, vmState := setupEnvironment(vm, received)
		h := &SyncKvvmHandler{client: fakeClient}

		done, err := h.syncMigrationReceiver(context.Background(), vmState, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(done).To(BeTrue())
		Expect(vmState.VirtualMachine().Changed().Annotations).NotTo(HaveKey(annotations.AnnVMMigrationReceiver))
	})
})

var _ = Describe("createReceiverIPAddress", func() {
	const (
		ns      = "app-x"
		address = "10.66.10.5"
	)

	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineKind},
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "vm-uid"},
		}
	}
	newReceiver := func() *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		return &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid"},
			Status:     intvirtv1alpha1.VirtualMachineReceiverStatus{Phase: intvirtv1alpha1.ReceiverPending},
		}
	}

	It("accepts the address created by a previous reconcile", func() {
		vm := newVM()
		receiver := newReceiver()
		own := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "web-ip", Namespace: ns, Labels: map[string]string{annotations.LabelVirtualMachineUID: string(vm.UID)}},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeStatic, StaticIP: address},
		}
		base, err := testutil.NewFakeClientWithObjects(vm, receiver, own)
		Expect(err).NotTo(HaveOccurred())
		h := &IPAMHandler{ipam: netmanager.NewIPAM(), client: denyVMIPCreateClient{base}, apiReader: base}

		Expect(h.createReceiverIPAddress(context.Background(), vm, receiver, address)).To(Succeed())
		Expect(receiver.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverPending))
	})

	It("fails the receiver when the address belongs to no address of this VM", func() {
		vm := newVM()
		receiver := newReceiver()
		base, err := testutil.NewFakeClientWithObjects(vm, receiver)
		Expect(err).NotTo(HaveOccurred())
		h := &IPAMHandler{ipam: netmanager.NewIPAM(), client: denyVMIPCreateClient{base}, apiReader: base}

		Expect(h.createReceiverIPAddress(context.Background(), vm, receiver, address)).To(Succeed())
		var got intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(base.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "web"}, &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
		Expect(got.Status.Reason).To(Equal(peering.ReceiverReasonIPAddressNotAvailable))
		Expect(got.Status.Message).To(ContainSubstring("the IP address is taken. Free the address there"))
		Expect(got.Status.Message).NotTo(ContainSubstring("admission webhook"))
	})
})

var _ = Describe("syncMigrationReceiver address", func() {
	It("fails the receiver when the source address is replaced by an Auto address", func() {
		const ns = "app-x"
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web", Namespace: ns, UID: "vm-uid",
				Annotations: map[string]string{annotations.AnnVMMigrationReceiver: "receiver-uid"},
			},
			Status: v1alpha2.VirtualMachineStatus{IPAddress: "10.66.10.1"},
		}
		receiver := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns, UID: "receiver-uid"},
			Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
				VirtualMachineTemplate:                       runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"web"},"spec":{}}`)},
				InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: []byte(`{"spec":{}}`)},
				IPAddress: "10.66.12.200",
			},
		}
		vmip := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-ip", Namespace: ns,
				Labels: map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"},
			},
			Spec:   v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
			Status: v1alpha2.VirtualMachineIPAddressStatus{Address: "10.66.10.1"},
		}
		fakeClient, _, vmState := setupEnvironment(vm, receiver, vmip)
		h := &SyncKvvmHandler{client: fakeClient}

		done, err := h.syncMigrationReceiver(context.Background(), vmState, receiver)
		Expect(err).NotTo(HaveOccurred())
		Expect(done).To(BeFalse())

		var got intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "web"}, &got)).To(Succeed())
		Expect(got.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
		Expect(got.Status.Reason).To(Equal(peering.ReceiverReasonIPAddressNotAvailable))
	})
})
