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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

// newKVVMWithMainInterfaceSpec builds an internal virtual machine whose template
// already advertises the Main interface spec: such a machine keeps it whatever the cluster
// answers now, so the handler owns its IPAddress.
func newKVVMWithMainInterfaceSpec(name, namespace string) *virtv1.VirtualMachine {
	specs := commonnetwork.InterfaceSpecList{{
		ID:             1,
		Type:           string(v1alpha2.NetworksTypeMain),
		InterfaceName:  commonnetwork.MainPodInterfaceName,
		IPAddressNames: []string{"vm-main-ip"},
		IPAddress:      "10.66.10.14",
	}}
	value, err := specs.ToString()
	Expect(err).NotTo(HaveOccurred())
	return &virtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: virtv1.VirtualMachineSpec{
			Template: &virtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{annotations.AnnNetworksSpec: value},
				},
			},
		},
	}
}

// newMainPrimaryCNINetwork creates the PrimaryCNI ClusterNetwork "main" with a
// pool — what makes the sdn module serve the Main network.
func newMainPrimaryCNINetwork() *unstructured.Unstructured {
	u := newReadyClusterNetworkWithPool(commonnetwork.MainClusterNetworkName, "d8-virtualization")
	Expect(unstructured.SetNestedField(u.Object, "PrimaryCNI", "spec", "type")).To(Succeed())
	return u
}

// withoutIPAddressKind answers the IPAddress requests the way an API server without
// the kinds of the sdn module does.
func withoutIPAddressKind(cl client.WithWatch) client.WithWatch {
	noMatch := &apimeta.NoKindMatchError{GroupKind: schema.GroupKind{Group: sdnapi.GroupName, Kind: "IPAddress"}}
	isIPAddress := func(obj runtime.Object) bool {
		switch obj.(type) {
		case *sdnapi.IPAddress, *sdnapi.IPAddressList:
			return true
		}
		return false
	}
	return interceptor.NewClient(cl, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if isIPAddress(obj) {
				return noMatch
			}
			return c.Get(ctx, key, obj, opts...)
		},
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if isIPAddress(list) {
				return noMatch
			}
			return c.List(ctx, list, opts...)
		},
	})
}

var _ = Describe("IPAMHandler for a VM on an IPAddress", func() {
	const (
		name      = "vm"
		namespace = "vms"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
		vm         *v1alpha2.VirtualMachine
		recorder   *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       "vm-uid",
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachinePending,
				// IPAMHandler acts only when the Main network is reflected in status.
				Networks: []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeMain}},
			},
		}
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...interface{}) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
		}
	})

	reconcile := func() {
		h := NewIPAMHandler(netmanager.NewIPAM(), fakeClient, recorder)
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.Update(context.Background())).To(Succeed())
	}

	getVM := func() *v1alpha2.VirtualMachine {
		newVM := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
		return newVM
	}

	getCondition := func() metav1.Condition {
		cond, exists := conditions.GetCondition(vmcondition.TypeIPAddressReady, getVM().Status.Conditions)
		Expect(exists).To(BeTrue())
		return cond
	}

	listVMIPs := func() []v1alpha2.VirtualMachineIPAddress {
		list := &v1alpha2.VirtualMachineIPAddressList{}
		Expect(fakeClient.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
		return list.Items
	}

	It("waits for the IPAddress to be created and does not create a vmip", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "vm-main"}}
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork())
		reconcile()

		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonIPAddressNotReady.String()))
		Expect(cond.Message).To(ContainSubstring("was not found"))
		Expect(listVMIPs()).To(BeEmpty())
	})

	It("waits while the IPAddress is not allocated (pool exhausted)", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "vm-main"}}
		ipAddr := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "", false)
		ipAddr.Status.Conditions = []metav1.Condition{{
			Type:   sdnapi.ConditionAllocated,
			Status: metav1.ConditionFalse,
			Reason: sdnapi.ReasonNoFreeIPAddress,
		}}
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(ContainSubstring("no free IP addresses"))
		Expect(getVM().Status.IPAddress).To(BeEmpty())
	})

	It("does not wait for an IPAddress whose address another IPAddress holds", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "vm-main"}}
		ipAddr := newSDNIPAddress("vm-main", namespace, "", "ClusterNetwork", "main", "", false)
		ipAddr.Status.Conditions = []metav1.Condition{{
			Type:   sdnapi.ConditionAllocated,
			Status: metav1.ConditionFalse,
			Reason: sdnapi.ReasonIPAddressIsUsed,
		}}
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonIPAddressNotAvailable.String()))
		Expect(cond.Message).To(ContainSubstring("another IPAddress already holds its address"))
	})

	It("reports the allocated address of the auto IPAddress in the status", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "vm-main"}}
		ipAddr := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonIPAddressReady.String()))
		newVM := getVM()
		Expect(newVM.Status.IPAddress).To(Equal("10.66.10.14"))
		Expect(newVM.Status.VirtualMachineIPAddress).To(BeEmpty())
		Expect(listVMIPs()).To(BeEmpty())
	})

	It("uses the static IPAddress referenced by spec.networks[Main].ipAddressName", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"}}
		ipAddr := newSDNIPAddress("my-static-ip", namespace, "", "ClusterNetwork", "main", "10.66.10.20", true)
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		Expect(getCondition().Status).To(Equal(metav1.ConditionTrue))
		Expect(getVM().Status.IPAddress).To(Equal("10.66.10.20"))
	})

	It("does not let a VM start again on a static IPAddress that is being deleted", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"}}
		vm.Status.Phase = v1alpha2.MachineRunning
		vm.Status.IPAddress = "10.66.10.20"
		ipAddr := newSDNIPAddress("my-static-ip", namespace, "", "ClusterNetwork", "main", "10.66.10.20", true)
		ipAddr.Status.UsedByPods = []string{namespace + "/virt-launcher-vm-abcde"}
		ipAddr.SetFinalizers([]string{"network.deckhouse.io/ipaddress"})
		ipAddr.SetDeletionTimestamp(ptr.To(metav1.Now()))
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse), "a restart would create the new pod on an address nothing holds")
		Expect(cond.Message).To(ContainSubstring("is being deleted"))
	})

	It("waits for a new auto IPAddress while the old one is being deleted", func() {
		ipAddr := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		ipAddr.SetFinalizers([]string{"network.deckhouse.io/ipaddress"})
		ipAddr.SetDeletionTimestamp(ptr.To(metav1.Now()))
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse), "a starting machine must not get an address that is about to be released")
		Expect(cond.Message).To(ContainSubstring("waiting for a new one"))
	})

	It("keeps a running VM ready on its auto IPAddress being deleted", func() {
		vm.Status.Phase = v1alpha2.MachineRunning
		vm.Status.IPAddress = "10.66.10.14"
		ipAddr := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		ipAddr.Status.UsedByPods = []string{namespace + "/virt-launcher-vm-abcde"}
		ipAddr.SetFinalizers([]string{"network.deckhouse.io/ipaddress"})
		ipAddr.SetDeletionTimestamp(ptr.To(metav1.Now()))
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		Expect(getCondition().Status).To(Equal(metav1.ConditionTrue), "the replacement is created once the pod is gone")
	})

	It("clears status.virtualMachineIPAddress after the explicit transition to a static IPAddress", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"}}
		vm.Status.VirtualMachineIPAddress = "old-vmip"
		ipAddr := newSDNIPAddress("my-static-ip", namespace, "", "ClusterNetwork", "main", "10.66.10.20", true)
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		Expect(getCondition().Status).To(Equal(metav1.ConditionTrue))
		Expect(getVM().Status.VirtualMachineIPAddress).To(BeEmpty())
	})

	It("clears status.virtualMachineIPAddress once the address comes from an IPAddress", func() {
		// The machine named a vmip and the owner removed the reference: the address
		// now comes from the IPAddress found by the machine label.
		kvvm := &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{},
		}}
		vm.Status.VirtualMachineIPAddress = "old-vmip"
		ipAddr := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		fakeClient, resource, vmState = setupEnvironment(vm, kvvm, ipAddr)
		reconcile()

		Expect(getCondition().Status).To(Equal(metav1.ConditionTrue))
		Expect(getVM().Status.VirtualMachineIPAddress).To(BeEmpty())
	})

	It("creates no vmip for a machine that has none: its address comes from an IPAddress", func() {
		kvvm := &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), kvvm)
		reconcile()

		Expect(listVMIPs()).To(BeEmpty(), "the deprecated resource is not created any more where the sdn module serves the Main network")
	})

	It("keeps the vmip of a machine that already has one", func() {
		kvvm := &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
		existing := newManagedVMIPForVM()
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), kvvm, existing)
		reconcile()

		vmips := listVMIPs()
		Expect(vmips).To(HaveLen(1))
		Expect(vmips[0].Name).To(Equal(existing.Name))
	})

	It("keeps a machine whose template already has the Main interface spec on its IPAddress", func() {
		// The pool is gone from the cluster, but the machine already runs on an
		// address of the sdn module: it must not be handed a vmip instead, or it
		// would end up with two addresses and a route to neither.
		own := newSDNIPAddress("vm-main-ip", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		fakeClient, resource, vmState = setupEnvironment(vm, newKVVMWithMainInterfaceSpec(name, namespace), own)
		reconcile()

		Expect(listVMIPs()).To(BeEmpty())
	})

	It("keeps the address on the vmip while the sdn module does not serve the Main network", func() {
		fakeClient, resource, vmState = setupEnvironment(vm)
		reconcile()

		Expect(listVMIPs()).To(HaveLen(1))
	})

	requeueAfter := func(cl client.Client) time.Duration {
		h := NewIPAMHandler(netmanager.NewIPAM(), cl, recorder)
		res, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.Update(context.Background())).To(Succeed())
		return res.RequeueAfter
	}

	It("blocks a VM naming the IPAddress another VM holds, and retries", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "shared-ip"}}
		holder := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "holder", Namespace: namespace, UID: "holder-uid"},
			Spec:       v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "shared-ip"}}},
			Status:     v1alpha2.VirtualMachineStatus{IPAddress: "10.66.10.20"},
		}
		ipAddr := newSDNIPAddress("shared-ip", namespace, "", "ClusterNetwork", "main", "10.66.10.20", true)
		var events []string
		recorder.EventFunc = func(_ client.Object, _, reason, _ string) { events = append(events, reason) }
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr, holder)

		Expect(requeueAfter(fakeClient)).To(Equal(addressConflictRetryInterval), "nothing links the two machines to wake this one up")
		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonIPAddressNotAvailable.String()))
		Expect(cond.Message).To(ContainSubstring(`"holder"`))
		Expect(events).To(ConsistOf(vmcondition.ReasonIPAddressNotAvailable.String()))
	})

	It("rejects an IPAddress of another network", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "corp-ip"}}
		ipAddr := newSDNIPAddress("corp-ip", namespace, "", "ClusterNetwork", "corp", "192.168.1.10", true)
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr)
		reconcile()

		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonIPAddressNotAvailable.String()))
		Expect(cond.Message).To(ContainSubstring("belongs to another network"))
	})

	It("waits for the IPAddress kind without an error", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "vm-main"}}
		fakeClient, resource, _ = setupEnvironment(vm, newMainPrimaryCNINetwork())
		cl := withoutIPAddressKind(fakeClient)
		vmState = state.New(cl, resource)

		Expect(requeueAfter(cl)).To(Equal(ipAddressKindRetryInterval), "no watch wakes the machine up while the kind is gone")
		cond := getCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(ContainSubstring("does not serve the IPAddress kind"))
	})

	newRunningKVVMI := func(guestIPs ...string) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Status: virtv1.VirtualMachineInstanceStatus{
				Phase:      virtv1.Running,
				Interfaces: []virtv1.VirtualMachineInstanceNetworkInterface{{Name: commonnetwork.NameDefaultInterface, IPs: guestIPs}},
			},
		}
	}

	DescribeTable("checks the address on the guest interface of a running VM",
		func(guestIPs []string, status metav1.ConditionStatus, reason vmcondition.IpAddressReadyReason) {
			vm.Status.Phase = v1alpha2.MachineRunning
			vm.Status.IPAddress = "10.66.10.14"
			ipAddr := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
			fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), ipAddr, newRunningKVVMI(guestIPs...))
			reconcile()

			cond := getCondition()
			Expect(cond.Status).To(Equal(status))
			Expect(cond.Reason).To(Equal(reason.String()))
		},
		Entry("the guest has the address", []string{"10.66.10.14"}, metav1.ConditionTrue, vmcondition.ReasonIPAddressReady),
		Entry("the guest has another address", []string{"10.66.10.99"}, metav1.ConditionFalse, vmcondition.ReasonIPAddressNotAssigned),
	)
})

var _ = Describe("IPAddressHandler for the Main network", func() {
	const (
		name      = "vm"
		namespace = "vms"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
		vm         *v1alpha2.VirtualMachine
	)

	BeforeEach(func() {
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: "vm-uid"},
			Status:     v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachinePending},
		}
	})

	reconcile := func() {
		h := NewIPAddressHandler(fakeClient, nil)
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.Update(context.Background())).To(Succeed())
	}

	listIPAddresses := func() []sdnapi.IPAddress {
		list := &sdnapi.IPAddressList{}
		Expect(fakeClient.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
		return list.Items
	}

	It("creates the auto Main IPAddress for a VM without a networks section", func() {
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), newKVVMWithMainInterfaceSpec(name, namespace))
		reconcile()

		items := listIPAddresses()
		Expect(items).To(HaveLen(1))
		ip := items[0]
		Expect(ip.Spec.NetworkRef.Kind).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
		Expect(ip.Spec.NetworkRef.Name).To(Equal(commonnetwork.MainClusterNetworkName))
		Expect(ip.GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
		Expect(ip.GetOwnerReferences()).To(HaveLen(1))
		Expect(ip.GetOwnerReferences()[0].Kind).To(Equal(v1alpha2.VirtualMachineKind))
	})

	// The address of a machine on a vmip comes from the vmip.
	It("does not create an IPAddress while the vmip holds the address", func() {
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), newManagedVMIPForVM())
		reconcile()

		Expect(listIPAddresses()).To(BeEmpty(), "the address of the machine comes from its vmip")
	})

	It("creates the IPAddress of a machine that has no vmip", func() {
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork())
		reconcile()

		Expect(listIPAddresses()).To(HaveLen(1))
	})

	It("keeps the address of the source for a VM received from another cluster", func() {
		vm.Annotations = map[string]string{annotations.AnnVMMigrationReceiver: "receiver-uid"}
		receiver := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: "receiver-uid"},
			Spec:       intvirtv1alpha1.VirtualMachineReceiverSpec{IPAddress: "10.66.10.1"},
		}
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), receiver)
		reconcile()

		items := listIPAddresses()
		Expect(items).To(HaveLen(1))
		Expect(items[0].Spec.Type).To(Equal(sdnapi.IPAddressTypeStatic))
		Expect(items[0].Spec.Static.IP).To(Equal("10.66.10.1"))
	})

	DescribeTable("fails the receive when this cluster cannot give the address of the source",
		func(ipa *sdnapi.IPAddress) {
			vm.Annotations = map[string]string{annotations.AnnVMMigrationReceiver: "receiver-uid"}
			receiver := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: "receiver-uid"},
				Spec:       intvirtv1alpha1.VirtualMachineReceiverSpec{IPAddress: "10.66.10.1"},
			}
			fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), receiver, ipa)
			reconcile()

			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(receiver), receiver)).To(Succeed())
			Expect(receiver.Status.Phase).To(Equal(intvirtv1alpha1.ReceiverFailed))
			Expect(receiver.Status.Message).To(ContainSubstring("10.66.10.1"))
		},
		Entry("another resource holds it", newSDNIPAddress("other", "other-ns", "other-uid", "ClusterNetwork", "main", "10.66.10.1", true)),
		Entry("the sdn module does not allocate it", func() *sdnapi.IPAddress {
			ipa := newSDNIPAddress("vm-a", namespace, "vm-uid", "ClusterNetwork", "main", "", false)
			ipa.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now().Add(-2 * receivedAddressAllocationWait))
			return ipa
		}()),
	)

	It("does not create an IPAddress for a static Main network", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"}}
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork())
		reconcile()

		Expect(listIPAddresses()).To(BeEmpty())
	})

	It("does not create an IPAddress while the sdn module does not serve the Main network", func() {
		fakeClient, resource, vmState = setupEnvironment(vm)
		reconcile()

		Expect(listIPAddresses()).To(BeEmpty())
	})

	It("deletes a duplicate of the Main IPAddress and keeps the oldest", func() {
		older := newSDNIPAddress("vm-b", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.15", true)
		older.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Minute))
		newer := newSDNIPAddress("vm-a", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		newer.CreationTimestamp = metav1.NewTime(time.Now())
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), older, newer)
		reconcile()

		items := listIPAddresses()
		Expect(items).To(HaveLen(1))
		Expect(items[0].Name).To(Equal("vm-b"))
	})

	// The address of such a machine comes from its vmip. A second resource for the same address only sits Pending
	// forever, and the arbitration then refuses to bind the vmip — the machine that owns
	// the address stops starting because of a resource we created for it ourselves.
	It("does not create an IPAddress for a machine that names its vmip", func() {
		vm.Spec.VirtualMachineIPAddress = "my-vmip"
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork())
		reconcile()

		Expect(listIPAddresses()).To(BeEmpty())
	})

	requeueAfter := func(cl client.Client) time.Duration {
		h := NewIPAddressHandler(cl, nil)
		res, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		return res.RequeueAfter
	}

	It("waits for the previous Main IPAddress to go away instead of creating another one", func() {
		previous := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		previous.SetFinalizers([]string{"network.deckhouse.io/ipaddress"})
		previous.SetDeletionTimestamp(ptr.To(metav1.Now()))
		fakeClient, resource, vmState = setupEnvironment(vm, newMainPrimaryCNINetwork(), previous)

		Expect(requeueAfter(fakeClient)).To(Equal(ipAddressTerminatingRetry))
		Expect(listIPAddresses()).To(HaveLen(1), "a replacement now would hold the address with the old one")
	})

	It("waits for the IPAddress kind without an error", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "vm-main"}}
		fakeClient, resource, _ = setupEnvironment(vm, newMainPrimaryCNINetwork())
		cl := withoutIPAddressKind(fakeClient)
		vmState = state.New(cl, resource)

		Expect(requeueAfter(cl)).To(Equal(ipAddressKindRetryInterval))
	})
})

// newManagedVMIPForVM is the vmip the controller created for the machine earlier:
// found by the virtual-machine-uid label, it is what keeps the address source on
// the deprecated resource.
func newManagedVMIPForVM() *v1alpha2.VirtualMachineIPAddress {
	return &v1alpha2.VirtualMachineIPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm-existing-ip",
			Namespace: "vms",
			Labels:    map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"},
		},
		Spec:   v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
		Status: v1alpha2.VirtualMachineIPAddressStatus{Address: "10.66.10.14"},
	}
}
