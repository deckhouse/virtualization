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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
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

// newSDNIPAddressUnstructured creates an SDN IPAddress unstructured for tests.
func newSDNIPAddressUnstructured(name, namespace, vmUID, networkKind, networkName, address string, allocated bool) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(commonnetwork.IPAddressGVK)
	u.SetName(name)
	u.SetNamespace(namespace)
	if vmUID != "" {
		u.SetLabels(map[string]string{annotations.LabelVirtualMachineUID: vmUID})
	}
	Expect(unstructured.SetNestedField(u.Object, map[string]any{"kind": networkKind, "name": networkName}, "spec", "networkRef")).To(Succeed())
	Expect(unstructured.SetNestedField(u.Object, "Auto", "spec", "type")).To(Succeed())
	if address != "" {
		Expect(unstructured.SetNestedField(u.Object, address, "status", "address")).To(Succeed())
	}
	condStatus := "False"
	phase := "Pending"
	if allocated {
		condStatus = "True"
		phase = "Allocated"
	}
	Expect(unstructured.SetNestedField(u.Object, phase, "status", "phase")).To(Succeed())
	Expect(unstructured.SetNestedSlice(u.Object, []any{
		map[string]any{"type": "Allocated", "status": condStatus, "reason": "IPAddressAllocated"},
	}, "status", "conditions")).To(Succeed())
	return u
}

var _ = Describe("collectIPAMErrors", func() {
	var (
		ctx       = testutil.ContextBackgroundWithNoOpLogger()
		namespace = "vms"
	)

	vmWithUID := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: namespace, UID: "vm-uid"}}
	}

	It("returns an error when ipAddressName is set but the network has no pool", func() {
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetwork("user-net", namespace))
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, namespace, vmWithUID(), []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "my-ip"},
		})

		Expect(errs).To(HaveLen(1))
		Expect(errs[0]).To(ContainSubstring("user-net"))
		Expect(errs[0]).To(ContainSubstring("my-ip"))
		Expect(errs[0]).To(ContainSubstring("no IPAM pool"))
	})

	It("returns no errors when static ipAddressName exists and matches the network", func() {
		ipAddr := newSDNIPAddressUnstructured("my-ip", namespace, "", "Network", "user-net", "192.168.201.10", true)
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace), ipAddr)
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, namespace, vmWithUID(), []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "my-ip"},
		})

		Expect(errs).To(BeEmpty())
	})

	It("returns an error when static ipAddressName does not exist", func() {
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace))
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, namespace, vmWithUID(), []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net", IPAddressName: "missing-ip"},
		})

		Expect(errs).To(HaveLen(1))
		Expect(errs[0]).To(ContainSubstring("missing-ip"))
		Expect(errs[0]).To(ContainSubstring("does not exist"))
	})

	It("returns no errors when auto IPAddress is allocated", func() {
		ipAddr := newSDNIPAddressUnstructured("vm-auto", namespace, "vm-uid", "Network", "user-net", "192.168.201.10", true)
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace), ipAddr)
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, namespace, vmWithUID(), []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net"},
		})

		Expect(errs).To(BeEmpty())
	})

	It("returns an error when auto IPAddress is not allocated", func() {
		ipAddr := newSDNIPAddressUnstructured("vm-auto", namespace, "vm-uid", "Network", "user-net", "", false)
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetworkWithPool(namespace), ipAddr)
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, namespace, vmWithUID(), []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net"},
		})

		Expect(errs).To(HaveLen(1))
		Expect(errs[0]).To(ContainSubstring("not yet allocated"))
	})

	It("returns no errors for network without pool and without ipAddressName", func() {
		cl, err := testutil.NewFakeClientWithObjects(newReadyNetwork("user-net", namespace))
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, namespace, vmWithUID(), []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeNetwork, Name: "user-net"},
		})

		Expect(errs).To(BeEmpty())
	})

	It("ignores the Main network", func() {
		cl, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		errs := collectIPAMErrors(ctx, cl, namespace, vmWithUID(), []v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeMain, IPAddressName: "ignored"},
		})

		Expect(errs).To(BeEmpty())
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

		staticIP := newSDNIPAddressUnstructured("my-ip", namespace, "", "ClusterNetwork", "corp-net", "192.168.200.42", true)
		newVM, _ := reconcile(newReadyClusterNetworkWithPool("corp-net", "demo-pool"), staticIP)

		cond, exists := conditions.GetCondition(vmcondition.TypeNetworkReady, newVM.Status.Conditions)
		Expect(exists).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(vmcondition.ReasonNetworkReady.String()))
	})
})
