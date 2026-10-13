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
	"errors"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

func TestMain(m *testing.M) {
	testutil.EnableSDNFeatureGate()
	os.Exit(m.Run())
}

func TestStepHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VMIP Steps Suite")
}

func newStepFakeClient(objs ...client.Object) client.Client {
	return newStepFakeClientWithInterceptor(interceptor.Funcs{}, objs...)
}

func newStepFakeClientWithInterceptor(funcs interceptor.Funcs, objs ...client.Object) client.Client {
	scheme := apiruntime.NewScheme()
	Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	Expect(virtv1.AddToScheme(scheme)).To(Succeed())
	Expect(sdnapi.AddToScheme(scheme)).To(Succeed())
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(funcs).Build()
}

func newMainClusterNetworkWithPool() *sdnapi.ClusterNetwork {
	return &sdnapi.ClusterNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: commonnetwork.MainClusterNetworkName},
		Spec: sdnapi.ClusterNetworkSpec{
			Type: sdnapi.ClusterNetworkTypePrimaryCNI,
			IPAM: &sdnapi.IPAMSpec{IPAddressPoolRef: &sdnapi.IPAddressPoolRef{Kind: sdnapi.ClusterIPAddressPoolKind, Name: "demo-pool"}},
		},
	}
}

func newStaticIPAddress(name, address string, allocated metav1.Condition) *sdnapi.IPAddress {
	allocated.Type = sdnapi.ConditionAllocated
	return &sdnapi.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Spec: sdnapi.IPAddressSpec{
			NetworkRef: sdnapi.NetworkRef{Kind: v1alpha2.NetworksTypeClusterNetwork, Name: commonnetwork.MainClusterNetworkName},
			Type:       sdnapi.IPAddressTypeStatic,
			Static:     &sdnapi.StaticIPAddress{IP: address},
		},
		Status: sdnapi.IPAddressStatus{Conditions: []metav1.Condition{allocated}},
	}
}

func newAllocatedIPAddress(name, address string) *sdnapi.IPAddress {
	ipa := newStaticIPAddress(name, address, metav1.Condition{Status: metav1.ConditionTrue, Reason: "IPAddressAllocated"})
	ipa.Status.Address = address
	return ipa
}

var _ = Describe("MirrorStep", func() {
	const (
		address   = "10.10.10.10"
		namespace = "ns"
	)

	var (
		ctx      context.Context
		vmip     *v1alpha2.VirtualMachineIPAddress
		lease    *v1alpha2.VirtualMachineIPAddressLease
		cb       *conditions.ConditionBuilder
		recorder *eventrecord.EventRecorderLoggerMock
		events   []string
	)

	BeforeEach(func() {
		ctx = context.Background()
		vmip = &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "vmip", Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: address},
		}
		lease = &v1alpha2.VirtualMachineIPAddressLease{
			ObjectMeta: metav1.ObjectMeta{Name: ip.IPToLeaseName(address)},
		}
		cb = conditions.NewConditionBuilder(vmipcondition.BoundType).Generation(vmip.Generation)
		events = nil
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, reason, _ string) { events = append(events, reason) },
			EventfFunc: func(_ client.Object, _, reason, _ string, _ ...interface{}) { events = append(events, reason) },
		}
	})

	// newRefusedIPAddress is refused the address the way the sdn module refuses one
	// another resource holds a lease on.
	newRefusedIPAddress := func() *sdnapi.IPAddress {
		return newStaticIPAddress("mirror", address, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  sdnapi.ReasonIPAddressIsUsed,
			Message: "IP " + address + " is reserved by IPAddressLease stale-lease",
		})
	}

	listIPAddresses := func(cl client.Client) []sdnapi.IPAddress {
		ipas := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, ipas, client.InNamespace(namespace))).To(Succeed())
		return ipas.Items
	}

	It("is skipped where the sdn module does not serve the Main network", func() {
		cl := newStepFakeClient()

		result, err := NewMirrorStep(nil, cl, cb, recorder).Take(ctx, vmip)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(listIPAddresses(cl)).To(BeEmpty())
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionUnknown))
	})

	It("sets the lease address in the status before the arbitration stops the chain", func() {
		// Regression: without the address in the status nothing can match this
		// vmip to its mirror, and the vmip hangs Pending forever.
		vmip.Status.Address = ""
		cl := newStepFakeClient(newMainClusterNetworkWithPool())

		result, err := NewMirrorStep(lease, cl, cb, recorder).Take(ctx, vmip)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(vmip.Status.Address).To(Equal(address))
	})

	It("mirrors a fresh vmip whose address only the lease takes", func() {
		// Regression: status.address is empty until BindStep, the mirror must be
		// created from the lease address, not stall on an empty name.
		vmip.Status.Address = ""
		cl := newStepFakeClient(newMainClusterNetworkWithPool())

		result, err := NewMirrorStep(lease, cl, cb, recorder).Take(ctx, vmip)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		mirrors := listIPAddresses(cl)
		Expect(mirrors).To(HaveLen(1))
		Expect(mirrors[0].Spec.Static.IP).To(Equal(address))
	})

	It("creates the mirror and keeps Bound off until it is allocated", func() {
		cl := newStepFakeClient(newMainClusterNetworkWithPool())

		result, err := NewMirrorStep(nil, cl, cb, recorder).Take(ctx, vmip)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		mirrors := listIPAddresses(cl)
		Expect(mirrors).To(HaveLen(1))
		Expect(mirrors[0].Spec.Static.IP).To(Equal(address))
		cond := cb.Condition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmipcondition.VirtualMachineIPAddressLeaseNotReady.String()))
		Expect(cond.Message).To(ContainSubstring("Waiting for the sdn module to allocate the address"))
	})

	It("continues the chain when the mirror is allocated", func() {
		cl := newStepFakeClient(
			newMainClusterNetworkWithPool(),
			newAllocatedIPAddress("mirror", address),
		)

		result, err := NewMirrorStep(nil, cl, cb, recorder).Take(ctx, vmip)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionUnknown))
	})

	It("releases the lease of an Auto vmip that is not Bound yet on an address conflict", func() {
		cl := newStepFakeClient(
			newMainClusterNetworkWithPool(),
			newRefusedIPAddress(),
			lease,
		)

		result, err := NewMirrorStep(lease, cl, cb, recorder).Take(ctx, vmip)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(vmip.Status.Address).To(BeEmpty())
		err = cl.Get(ctx, types.NamespacedName{Name: lease.Name}, &v1alpha2.VirtualMachineIPAddressLease{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue())
		cond := cb.Condition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(ContainSubstring("picking another address"))
		Expect(events).NotTo(BeEmpty())
	})

	Context("on a vmip the controller created for a VM", func() {
		const vmName = "vm"

		// makeVMIPOfVM makes the vmip one the controller created for the VM.
		makeVMIPOfVM := func() {
			vmip.Labels = map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"}
			vmip.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineKind,
				Name:       vmName,
				UID:        "vm-uid",
			}}
		}

		newVM := func(vmAddress string) *v1alpha2.VirtualMachine {
			return &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: "vm-uid"},
				Status:     v1alpha2.VirtualMachineStatus{IPAddress: vmAddress},
			}
		}

		It("mirrors the vmip of a VM moved to an IPAddress without the VM label and does not hold Bound back", func() {
			makeVMIPOfVM()
			cl := newStepFakeClient(newMainClusterNetworkWithPool(), newVM("10.10.10.99"))

			result, err := NewMirrorStep(nil, cl, cb, recorder).Take(ctx, vmip)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "a vmip the VM does not run on must not wait for the mirror allocation")
			mirrors := listIPAddresses(cl)
			Expect(mirrors).To(HaveLen(1))
			mirror := mirrors[0]
			Expect(mirror.GetName()).To(Equal(vmip.Name))
			Expect(mirror.GetLabels()).NotTo(HaveKey(annotations.LabelVirtualMachineUID))
			Expect(mirror.GetOwnerReferences()).To(HaveLen(1))
			Expect(mirror.GetOwnerReferences()[0].Kind).To(Equal(v1alpha2.VirtualMachineIPAddressKind))
			Expect(cb.Condition().Status).To(Equal(metav1.ConditionUnknown))
		})

		It("labels the mirror with the VM that runs on the vmip address", func() {
			makeVMIPOfVM()
			cl := newStepFakeClient(newMainClusterNetworkWithPool(), newVM(address))

			result, err := NewMirrorStep(nil, cl, cb, recorder).Take(ctx, vmip)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(), "the address source vmip waits for the mirror allocation")
			mirrors := listIPAddresses(cl)
			Expect(mirrors).To(HaveLen(1))
			mirror := mirrors[0]
			Expect(mirror.GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
			Expect(mirror.GetOwnerReferences()).To(HaveLen(1))
			Expect(mirror.GetOwnerReferences()[0].Kind).To(Equal(v1alpha2.VirtualMachineKind))
		})
	})

	Context("on a vmip that is Bound already", func() {
		markVMIPBound := func() {
			vmip.Status.Conditions = []metav1.Condition{{
				Type:   vmipcondition.BoundType.String(),
				Status: metav1.ConditionTrue,
				Reason: vmipcondition.Bound.String(),
			}}
			lease.Spec.VirtualMachineIPAddressRef = &v1alpha2.VirtualMachineIPAddressLeaseIpAddressRef{Name: vmip.Name, Namespace: vmip.Namespace}
		}

		It("keeps it Bound when the sdn module refuses the address", func() {
			markVMIPBound()
			cl := newStepFakeClient(newMainClusterNetworkWithPool(), newRefusedIPAddress(), lease)

			result, err := NewMirrorStep(lease, cl, cb, recorder).Take(ctx, vmip)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			cond := cb.Condition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Message).To(ContainSubstring("is reserved by IPAddressLease stale-lease"))
			Expect(events).To(BeEmpty())
			Expect(cl.Get(ctx, types.NamespacedName{Name: lease.Name}, &v1alpha2.VirtualMachineIPAddressLease{})).To(Succeed())
		})

		It("keeps it Bound when the name of the mirror is taken", func() {
			markVMIPBound()
			ipaWithMirrorName := newAllocatedIPAddress(vmip.Name, "10.10.10.99")
			cl := newStepFakeClient(newMainClusterNetworkWithPool(), ipaWithMirrorName, lease)

			result, err := NewMirrorStep(lease, cl, cb, recorder).Take(ctx, vmip)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RequeueAfter).To(Equal(mirrorNameTakenRetry))
			Expect(cb.Condition().Status).To(Equal(metav1.ConditionTrue))
			Expect(cb.Condition().Message).To(ContainSubstring(vmip.Name))
		})

		It("does not keep it Bound once the lease no longer backs it", func() {
			markVMIPBound()
			lease.Spec.VirtualMachineIPAddressRef = &v1alpha2.VirtualMachineIPAddressLeaseIpAddressRef{Name: "another-vmip", Namespace: vmip.Namespace}
			cl := newStepFakeClient(newMainClusterNetworkWithPool(), newRefusedIPAddress(), lease)

			_, err := NewMirrorStep(lease, cl, cb, recorder).Take(ctx, vmip)

			Expect(err).NotTo(HaveOccurred())
			Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
		})
	})

	It("only reports the conflict for a static vmip and keeps the lease", func() {
		vmip.Spec.Type = v1alpha2.VirtualMachineIPAddressTypeStatic
		vmip.Spec.StaticIP = address
		cl := newStepFakeClient(
			newMainClusterNetworkWithPool(),
			newRefusedIPAddress(),
			lease,
		)

		result, err := NewMirrorStep(lease, cl, cb, recorder).Take(ctx, vmip)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(vmip.Status.Address).To(Equal(address))
		Expect(cl.Get(ctx, types.NamespacedName{Name: lease.Name}, &v1alpha2.VirtualMachineIPAddressLease{})).To(Succeed())
		cond := cb.Condition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmipcondition.VirtualMachineIPAddressLeaseNotReady.String()))
		Expect(cond.Message).To(ContainSubstring("is reserved by IPAddressLease stale-lease"))
	})
})

var _ = Describe("MirrorStep on a deletingVMIP vmip", func() {
	const (
		address   = "10.10.10.10"
		namespace = "ns"
	)

	It("creates no mirror: nothing would ever delete it", func() {
		ctx := context.Background()
		deletingVMIP := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vmip-deletingVMIP",
				Namespace:         namespace,
				DeletionTimestamp: ptr.To(metav1.Now()),
				Finalizers:        []string{v1alpha2.FinalizerIPAddressCleanup},
			},
			Spec:   v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
			Status: v1alpha2.VirtualMachineIPAddressStatus{Address: address},
		}
		cl := newStepFakeClient(newMainClusterNetworkWithPool())
		cb := conditions.NewConditionBuilder(vmipcondition.BoundType)
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}

		result, err := NewMirrorStep(nil, cl, cb, recorder).Take(ctx, deletingVMIP)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())

		ipas := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, ipas, client.InNamespace(namespace))).To(Succeed())
		Expect(ipas.Items).To(BeEmpty(), "a mirror created after the cleanup keeps the address forever")
	})
})

var _ = Describe("MirrorStep in a terminating namespace", func() {
	const (
		address   = "10.10.10.10"
		namespace = "ns"
	)

	It("ends the reconcile instead of retrying what admission will never allow", func() {
		ctx := context.Background()
		vmip := &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: "vmip", Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
			Status:     v1alpha2.VirtualMachineIPAddressStatus{Address: address},
		}
		cl := newStepFakeClientWithInterceptor(
			interceptor.Funcs{
				Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
					return k8serrors.NewForbidden(schema.GroupResource{Resource: "ipaddresses"}, obj.GetName(),
						errors.New("unable to create new content in namespace ns because it is being terminated"))
				},
			},
			newMainClusterNetworkWithPool(),
		)
		cb := conditions.NewConditionBuilder(vmipcondition.BoundType)
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}

		result, err := NewMirrorStep(nil, cl, cb, recorder).Take(ctx, vmip)

		Expect(err).NotTo(HaveOccurred(), "a namespace being torn down is not a controller failure")
		Expect(result).NotTo(BeNil())
		Expect(cb.Condition().Message).To(ContainSubstring("Namespace is terminating"))
	})
})
