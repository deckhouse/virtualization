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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

// newReadyNetworkWithPool creates a namespaced Network with a Ready condition and
// an IPAM pool reference (spec.ipam.ipAddressPoolRef).
func newReadyNetworkWithPool(namespace string) *unstructured.Unstructured {
	u := newReadyNetwork("user-net", namespace)
	Expect(unstructured.SetNestedField(u.Object, map[string]any{
		"ipAddressPoolRef": map[string]any{
			"kind": "IPAddressPool",
			"name": "demo-pool",
		},
	}, "spec", "ipam")).To(Succeed())
	return u
}

// newReadyClusterNetworkWithPool creates a cluster-scoped ClusterNetwork with a
// Ready condition and an IPAM pool reference.
func newReadyClusterNetworkWithPool(name, poolName string) *unstructured.Unstructured {
	u := newReadyClusterNetwork(name)
	Expect(unstructured.SetNestedField(u.Object, map[string]any{
		"ipAddressPoolRef": map[string]any{
			"kind": "ClusterIPAddressPool",
			"name": poolName,
		},
	}, "spec", "ipam")).To(Succeed())
	return u
}

// newReadyClusterNetwork creates a cluster-scoped ClusterNetwork with a Ready condition.
func newReadyClusterNetwork(name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(commonnetwork.ClusterNetworkGVK)
	u.SetName(name)
	Expect(unstructured.SetNestedSlice(u.Object, []interface{}{
		map[string]interface{}{
			"type":   "Ready",
			"status": "True",
		},
	}, "status", "conditions")).To(Succeed())
	return u
}

// newSDNIPAddress creates an SDN IPAddress for tests.
func newSDNIPAddress(name, namespace, vmUID, networkKind, networkName, address string, allocated bool) *sdnapi.IPAddress {
	ipa := &sdnapi.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: sdnapi.IPAddressSpec{
			NetworkRef: sdnapi.NetworkRef{Kind: networkKind, Name: networkName},
			Type:       sdnapi.IPAddressTypeAuto,
		},
		Status: sdnapi.IPAddressStatus{Address: address},
	}
	if vmUID != "" {
		ipa.SetLabels(map[string]string{annotations.LabelVirtualMachineUID: vmUID})
	}
	condStatus := metav1.ConditionFalse
	ipa.Status.Phase = sdnapi.PhasePending
	if allocated {
		condStatus = metav1.ConditionTrue
		ipa.Status.Phase = sdnapi.PhaseAllocated
	}
	ipa.Status.Conditions = []metav1.Condition{{
		Type:   sdnapi.ConditionAllocated,
		Status: condStatus,
		Reason: "IPAddressAllocated",
	}}
	return ipa
}

var _ = Describe("collectIPAMErrors", func() {
	var (
		ctx       = testutil.ContextBackgroundWithNoOpLogger()
		namespace = "vms"
	)

	vmWithNetworks := func(networks ...v1alpha2.NetworksSpec) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: namespace, UID: "vm-uid"},
			Spec:       v1alpha2.VirtualMachineSpec{Networks: networks},
		}
	}

	It("returns an error when ipAddressName is set but the network has no pool", func() {
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetwork("user-net", namespace))
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, vmWithNetworks(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "my-ip"},
		))

		Expect(errs).To(HaveLen(1))
		Expect(errs[0]).To(ContainSubstring("user-net"))
		Expect(errs[0]).To(ContainSubstring("my-ip"))
		Expect(errs[0]).To(ContainSubstring("no IPAM pool"))
	})

	It("returns no errors when static ipAddressName exists and matches the network", func() {
		ipAddr := newSDNIPAddress("my-ip", namespace, "", "Network", "user-net", "192.168.201.10", true)
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace), ipAddr)
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, vmWithNetworks(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "my-ip"},
		))

		Expect(errs).To(BeEmpty())
	})

	It("returns an error when static ipAddressName does not exist", func() {
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace))
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, vmWithNetworks(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "missing-ip"},
		))

		Expect(errs).To(HaveLen(1))
		Expect(errs[0]).To(ContainSubstring("missing-ip"))
		Expect(errs[0]).To(ContainSubstring("does not exist"))
	})

	It("returns no errors when auto IPAddress is allocated", func() {
		ipAddr := newSDNIPAddress("vm-auto", namespace, "vm-uid", "Network", "user-net", "192.168.201.10", true)
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace), ipAddr)
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, vmWithNetworks(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net"},
		))

		Expect(errs).To(BeEmpty())
	})

	It("returns an error when auto IPAddress is not allocated", func() {
		ipAddr := newSDNIPAddress("vm-auto", namespace, "vm-uid", "Network", "user-net", "", false)
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace), ipAddr)
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, vmWithNetworks(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net"},
		))

		Expect(errs).To(HaveLen(1))
		Expect(errs[0]).To(ContainSubstring("not yet allocated"))
	})

	It("reports an auto IPAddress being deleted as being replaced", func() {
		ipAddr := newSDNIPAddress("vm-auto", namespace, "vm-uid", "Network", "user-net", "192.168.201.10", true)
		ipAddr.SetFinalizers([]string{"network.deckhouse.io/ipaddress"})
		ipAddr.SetDeletionTimestamp(ptr.To(metav1.Now()))
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace), ipAddr)
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, vmWithNetworks(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net"},
		))

		Expect(errs).To(HaveLen(1))
		Expect(errs[0]).To(ContainSubstring("is being replaced"))
	})

	It("returns no errors for network without pool and without ipAddressName", func() {
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetwork("user-net", namespace))
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, vmWithNetworks(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net"},
		))

		Expect(errs).To(BeEmpty())
	})

	It("ignores the Main network", func() {
		cl, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, vmWithNetworks(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain, IPAddressName: "ignored"},
		))

		Expect(errs).To(BeEmpty())
	})
})

var _ = Describe("IPAddressHandler.cleanupOrphanedIPAddresses", func() {
	var (
		ctx       = testutil.ContextBackgroundWithNoOpLogger()
		namespace = "vms"
	)

	newVM := func(networks ...v1alpha2.NetworksSpec) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: namespace, UID: "vm-uid"},
			Spec:       v1alpha2.VirtualMachineSpec{Networks: networks},
		}
	}

	listIPAddresses := func(cl client.Client) []sdnapi.IPAddress {
		list := &sdnapi.IPAddressList{}
		Expect(cl.List(ctx, list)).To(Succeed())
		return list.Items
	}
	cleanup := func(cl client.Client, vm *v1alpha2.VirtualMachine, mainInterfaceSpecEnabled bool) error {
		return NewIPAddressHandler(cl, nil).cleanupOrphanedIPAddresses(ctx, vm, listIPAddresses(cl), mainInterfaceSpecEnabled)
	}

	It("keeps the auto Main IPAddress while the Main interface spec is enabled", func() {
		mainIP := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		cl, err := testutil.NewFakeClientWithObjects(mainIP)
		Expect(err).NotTo(HaveOccurred())

		Expect(cleanup(cl, newVM(), true)).To(Succeed())

		Expect(listIPAddresses(cl)).To(HaveLen(1))
	})

	It("keeps the Main IPAddress of a VM on a vmip: it is not this handler's to collect", func() {
		mainIP := newSDNIPAddress("vm-main", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		cl, err := testutil.NewFakeClientWithObjects(mainIP)
		Expect(err).NotTo(HaveOccurred())

		Expect(cleanup(cl, newVM(), false)).To(Succeed())

		Expect(listIPAddresses(cl)).To(HaveLen(1))
	})

	It("keeps the mirror of a new vmip while the pod runs on the mirror of the previous one", func() {
		previous := newSDNIPAddress("vmip-auto", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.40", true)
		previous.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: "vmip-auto-uid"})
		previous.Status.UsedByPods = []string{namespace + "/virt-launcher-vm-abcde"}
		current := newSDNIPAddress("vmip-static", namespace, "vm-uid", "ClusterNetwork", "main", "", false)
		current.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: "vmip-static-uid"})
		cl, err := testutil.NewFakeClientWithObjects(previous, current)
		Expect(err).NotTo(HaveOccurred())

		vm := newVM()
		vm.Status.Phase = v1alpha2.MachineRunning
		vm.Status.IPAddress = "10.66.10.40"
		Expect(cleanup(cl, vm, true)).To(Succeed())

		Expect(listIPAddresses(cl)).To(HaveLen(2))
	})

	It("keeps a labeled IPAddress referenced by spec.networks[Main].ipAddressName", func() {
		refIP := newSDNIPAddress("my-static-ip", namespace, "vm-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		cl, err := testutil.NewFakeClientWithObjects(refIP)
		Expect(err).NotTo(HaveOccurred())

		vm := newVM(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-static-ip"})
		Expect(cleanup(cl, vm, true)).To(Succeed())

		items := listIPAddresses(cl)
		Expect(items).To(HaveLen(1))
		Expect(items[0].GetName()).To(Equal("my-static-ip"))
	})

	It("keeps a labeled IPAddress referenced by an additional network's ipAddressName and removes the orphaned one", func() {
		refIP := newSDNIPAddress("corp-static", namespace, "vm-uid", "ClusterNetwork", "corp-net", "192.168.200.4", true)
		orphanedIP := newSDNIPAddress("vm-old", namespace, "vm-uid", "ClusterNetwork", "old-net", "192.168.200.5", true)
		cl, err := testutil.NewFakeClientWithObjects(refIP, orphanedIP)
		Expect(err).NotTo(HaveOccurred())

		vm := newVM(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain},
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net", IPAddressName: "corp-static"},
		)
		Expect(cleanup(cl, vm, false)).To(Succeed())

		items := listIPAddresses(cl)
		Expect(items).To(HaveLen(1))
		Expect(items[0].GetName()).To(Equal("corp-static"))
	})

	It("removes the auto IPAddress of an additional network removed from spec and keeps the wanted one", func() {
		removedIP := newSDNIPAddress("vm-old", namespace, "vm-uid", "ClusterNetwork", "old-net", "192.168.200.4", true)
		wantedIP := newSDNIPAddress("vm-corp", namespace, "vm-uid", "ClusterNetwork", "corp-net", "192.168.200.5", true)
		cl, err := testutil.NewFakeClientWithObjects(removedIP, wantedIP)
		Expect(err).NotTo(HaveOccurred())

		vm := newVM(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain},
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"},
		)
		Expect(cleanup(cl, vm, false)).To(Succeed())

		items := listIPAddresses(cl)
		Expect(items).To(HaveLen(1))
		Expect(items[0].GetName()).To(Equal("vm-corp"))
	})
})

var _ = Describe("extractIPAddressesFromPods", func() {
	It("returns name to address map from ipAddressConfigs", func() {
		pods := &corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod",
				Namespace: "ns",
				Annotations: map[string]string{
					"network.deckhouse.io/networks-status": `[
						{"type":"ClusterNetwork","name":"corp-net","ifName":"veth_cn1","mac":"aa:bb:cc:dd:ee:01","ipAddressConfigs":[{"name":"ip1","address":"192.168.200.4","network":"192.168.200.0/24"}]},
						{"type":"Network","name":"user-net","ifName":"veth_n1","mac":"aa:bb:cc:dd:ee:02","ipAddressConfigs":[{"name":"ip2","address":"192.168.201.10","network":"192.168.201.0/24"}]}
					]`,
				},
			},
		}}}

		result := extractIPAddressesFromPods(pods)

		Expect(result).To(HaveKeyWithValue("corp-net", "192.168.200.4"))
		Expect(result).To(HaveKeyWithValue("user-net", "192.168.201.10"))
	})

	It("returns empty map when annotation is absent", func() {
		pods := &corev1.PodList{Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns"}}}}

		result := extractIPAddressesFromPods(pods)

		Expect(result).To(BeEmpty())
	})

	It("ignores interfaces without ipAddressConfigs", func() {
		pods := &corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod",
				Namespace: "ns",
				Annotations: map[string]string{
					"network.deckhouse.io/networks-status": `[
						{"type":"ClusterNetwork","name":"corp-net","ifName":"veth_cn1","mac":"aa:bb:cc:dd:ee:01"}
					]`,
				},
			},
		}}}

		result := extractIPAddressesFromPods(pods)

		Expect(result).To(BeEmpty())
	})

	It("ignores malformed annotation", func() {
		pods := &corev1.PodList{Items: []corev1.Pod{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod",
				Namespace: "ns",
				Annotations: map[string]string{
					"network.deckhouse.io/networks-status": `not-json`,
				},
			},
		}}}

		result := extractIPAddressesFromPods(pods)

		Expect(result).To(BeEmpty())
	})

	It("handles nil pods", func() {
		result := extractIPAddressesFromPods(nil)
		Expect(result).To(BeEmpty())
	})
})

var _ = Describe("NetworkInterfaceHandler IPAM integration", func() {
	const (
		name      = "vm"
		namespace = "vms"
		podName   = "test-pod"
	)
	var (
		ctx   = testutil.ContextBackgroundWithNoOpLogger()
		vm    *v1alpha2.VirtualMachine
		vmPod *corev1.Pod
		mac1  *v1alpha2.VirtualMachineMACAddress
	)

	BeforeEach(func() {
		vmPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        podName,
				Namespace:   namespace,
				Annotations: map[string]string{},
				Labels:      map[string]string{virtv1.VirtualMachineNameLabel: name},
			},
		}
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: "test-uid"},
		}
		mac1 = &v1alpha2.VirtualMachineMACAddress{
			TypeMeta:   metav1.TypeMeta{Kind: "VirtualMachineMACAddress", APIVersion: "virtualization.deckhouse.io/v1alpha2"},
			ObjectMeta: metav1.ObjectMeta{Name: "mac1", Namespace: namespace, Labels: map[string]string{annotations.LabelVirtualMachineUID: string(vm.UID)}},
			Status:     v1alpha2.VirtualMachineMACAddressStatus{Address: "aa:bb:cc:dd:ee:ff", Phase: v1alpha2.VirtualMachineMACAddressPhaseAttached, VirtualMachine: name},
		}
	})

	reconcile := func(networkObjs ...client.Object) (*v1alpha2.VirtualMachine, client.WithWatch) {
		allObjs := []client.Object{vmPod, mac1}
		allObjs = append(allObjs, networkObjs...)
		fakeClient, resource, vmState := setupEnvironment(vm, allObjs...)
		gate, _, setFromMap, err := featuregates.New()
		Expect(err).NotTo(HaveOccurred())
		Expect(setFromMap(map[string]bool{string(featuregates.SDN): true})).To(Succeed())
		h := NewNetworkInterfaceHandler(gate, nil)
		_, err = h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.Update(context.Background())).To(Succeed())

		newVM := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)).To(Succeed())
		return newVM, fakeClient
	}

	It("populates status.networks[].ipAddress from networks-status ipAddressConfigs", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeMain},
			{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"},
		}
		vmPod.Annotations["network.deckhouse.io/networks-status"] = `[{"type":"ClusterNetwork","name":"corp-net","ifName":"veth_cn1","mac":"aa:bb:cc:dd:ee:ff","ipAddressConfigs":[{"name":"ip1","address":"192.168.200.4","network":"192.168.200.0/24"}],"conditions":[{"type":"Configured","status":"True","reason":"InterfaceConfiguredSuccessfully","message":""},{"type":"Negotiated","status":"True","reason":"Up","message":""}]}]`

		newVM, _ := reconcile(newReadyClusterNetworkWithPool("corp-net", "demo-pool"))

		var corpStatus *v1alpha2.NetworksStatus
		for i := range newVM.Status.Networks {
			if newVM.Status.Networks[i].Name == "corp-net" {
				corpStatus = &newVM.Status.Networks[i]
			}
		}
		Expect(corpStatus).NotTo(BeNil(), "corp-net status entry should exist")
		Expect(corpStatus.IPAddress).To(Equal("192.168.200.4"))
	})

	It("fills the Main status entry with the SDN IPAddress name and address", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeMain},
			{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net"},
		}
		vmPod.Annotations["network.deckhouse.io/networks-status"] = `[{"type":"ClusterNetwork","name":"corp-net","ifName":"veth_cn1","mac":"aa:bb:cc:dd:ee:ff","conditions":[{"type":"Configured","status":"True","reason":"InterfaceConfiguredSuccessfully","message":""},{"type":"Negotiated","status":"True","reason":"Up","message":""}]}]`

		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: map[string]string{},
			},
			Spec: virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{}},
		}
		mainIP := newSDNIPAddress("vm-main", namespace, "test-uid", "ClusterNetwork", "main", "10.66.10.14", true)
		newVM, _ := reconcile(kvvm, newReadyClusterNetwork("corp-net"), mainIP)

		var mainStatus *v1alpha2.NetworksStatus
		for i := range newVM.Status.Networks {
			if newVM.Status.Networks[i].Type == v1alpha2.NetworksTypeMain {
				mainStatus = &newVM.Status.Networks[i]
			}
		}
		Expect(mainStatus).NotTo(BeNil(), "the Main status entry should exist")
		Expect(mainStatus.IPAddressName).To(Equal("vm-main"))
		Expect(mainStatus.IPAddress).To(Equal("10.66.10.14"))
	})

	It("aggregates ipAddressName-without-pool error into NetworkReady", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeMain},
			{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net", IPAddressName: "my-ip"},
		}
		vmPod.Annotations["network.deckhouse.io/networks-status"] = `[{"type":"ClusterNetwork","name":"corp-net","ifName":"veth_cn1","mac":"aa:bb:cc:dd:ee:ff","conditions":[{"type":"Configured","status":"True","reason":"InterfaceConfiguredSuccessfully","message":""},{"type":"Negotiated","status":"True","reason":"Up","message":""}]}]`

		newVM, _ := reconcile(newReadyClusterNetwork("corp-net"))

		cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, newVM.Status.Conditions)
		Expect(exists).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonNetworkNotReady.String()))
		Expect(cond.Message).To(ContainSubstring("corp-net"))
		Expect(cond.Message).To(ContainSubstring("my-ip"))
		Expect(cond.Message).To(ContainSubstring("no IPAM pool"))
	})

	It("does not report IPAM error when ipAddressName is set and pool exists", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeMain},
			{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "corp-net", IPAddressName: "my-ip"},
		}
		vmPod.Annotations["network.deckhouse.io/networks-status"] = `[{"type":"ClusterNetwork","name":"corp-net","ifName":"veth_cn1","mac":"aa:bb:cc:dd:ee:ff","conditions":[{"type":"Configured","status":"True","reason":"InterfaceConfiguredSuccessfully","message":""},{"type":"Negotiated","status":"True","reason":"Up","message":""}]}]`

		staticIP := newSDNIPAddress("my-ip", namespace, "", "ClusterNetwork", "corp-net", "192.168.200.42", true)
		newVM, _ := reconcile(newReadyClusterNetworkWithPool("corp-net", "demo-pool"), staticIP)

		cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, newVM.Status.Conditions)
		Expect(exists).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonNetworkReady.String()))
	})
})
