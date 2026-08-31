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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SyncKvvmHandler network detach corner cases", func() {
	const (
		vmName    = "vm-detach-edge"
		vmUID     = types.UID("vm-detach-edge-uid")
		namespace = "default"
		cnName    = "cnet-detach-edge"
		macName   = "vmmac-detach-edge"
		macAddr   = "aa:bb:cc:dd:ee:02"
		podName   = "d8v-detach-edge"
		nodeName  = "node-detach-edge"
		poolName  = "pool-detach-edge"
		ipName    = "ip-detach-edge"
		podUID    = types.UID("detach-edge-pod-uid")
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	mainNetwork := v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(commonnetwork.ReservedMainID)}
	additionalNetwork := v1alpha2.NetworksSpec{
		Type:                         v1alpha2.NetworksTypeClusterNetwork,
		Name:                         cnName,
		ID:                           ptr.To(2),
		VirtualMachineMACAddressName: macName,
	}

	newVM := func(networks ...v1alpha2.NetworksSpec) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: vmUID},
			Spec:       v1alpha2.VirtualMachineSpec{Networks: networks},
		}
	}

	mac := &v1alpha2.VirtualMachineMACAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      macName,
			Namespace: namespace,
			Labels:    map[string]string{annotations.LabelVirtualMachineUID: string(vmUID)},
		},
		Status: v1alpha2.VirtualMachineMACAddressStatus{
			Address: macAddr,
			Phase:   v1alpha2.VirtualMachineMACAddressPhaseAttached,
		},
	}

	// attachedSpecs is the interface list of the virtual machine while it still asks for
	// the additional network, that is what the pod annotation carries before the detach.
	attachedSpecs := commonnetwork.CreateNetworkSpec(
		newVM(mainNetwork, additionalNetwork),
		[]*v1alpha2.VirtualMachineMACAddress{mac},
	)

	additionalIfaceName := func() string {
		for _, spec := range attachedSpecs {
			if spec.Type != string(v1alpha2.NetworksTypeMain) {
				return spec.InterfaceName
			}
		}
		return ""
	}()

	newKVVM := func(ifaces ...virtv1.Interface) *virtv1.VirtualMachine {
		kvvm := newEmptyKVVM(vmName, namespace)
		kvvm.Spec.Template = &virtv1.VirtualMachineInstanceTemplateSpec{}
		kvvm.Spec.Template.Spec.Domain.Devices.Interfaces = ifaces
		return kvvm
	}

	newKVVMI := func() *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(vmName, namespace)
		kvvmi.Status.Phase = virtv1.Running
		kvvmi.Status.NodeName = nodeName
		kvvmi.Status.ActivePods = map[types.UID]string{podUID: nodeName}
		return kvvmi
	}

	newPod := func(anns map[string]string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        podName,
				Namespace:   namespace,
				UID:         podUID,
				Labels:      map[string]string{virtv1.VirtualMachineNameLabel: vmName},
				Annotations: anns,
			},
			Spec:   corev1.PodSpec{NodeName: nodeName},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}

	getPod := func(fakeClient client.WithWatch) *corev1.Pod {
		pod := &corev1.Pod{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, pod)).To(Succeed())
		return pod
	}

	Context("a launcher pod that was never annotated", func() {
		// A Main-only virtual machine whose last additional interface is still leaving
		// the domain: the pod has nothing SDN-managed, so it must be left alone. The
		// drift check and the patch have to agree on that, otherwise the drift is
		// reported on every reconcile while the patch keeps skipping the pod.
		var (
			fakeClient client.WithWatch
			vmState    state.VirtualMachineState
			kvvm       *virtv1.VirtualMachine
		)

		BeforeEach(func() {
			kvvm = newKVVM(
				virtv1.Interface{Name: commonnetwork.NameDefaultInterface},
				virtv1.Interface{Name: additionalIfaceName, State: virtv1.InterfaceStateAbsent},
			)
			fakeClient, _, vmState = setupEnvironment(
				newVM(mainNetwork), kvvm, newKVVMI(), mac,
				newReadyClusterNetwork(cnName), newPod(nil), newNodeWithTapSupport(nodeName),
			)
		})

		It("reports no drift", func() {
			h := &SyncKvvmHandler{client: fakeClient}

			outOfSync, err := h.networksOutOfSync(ctx, vmState, kvvm)
			Expect(err).NotTo(HaveOccurred())
			Expect(outOfSync).To(BeFalse())
		})

		It("is left without the networks-spec and the tap annotation", func() {
			h := &SyncKvvmHandler{client: fakeClient}

			_, err := h.patchPodNetworkAnnotation(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			Expect(getPod(fakeClient).Annotations).NotTo(HaveKey(annotations.AnnNetworksSpec))
			Expect(getPod(fakeClient).Annotations).NotTo(HaveKey(annotations.AnnTapProvisionByDVPSupported))
		})
	})

	Context("an additional network with an IPAM pool", func() {
		// The pod annotation and the KVVM template are both built from the IPAM-enriched
		// spec, so the drift check has to compare against the enriched one too.
		var (
			fakeClient client.WithWatch
			vmState    state.VirtualMachineState
			kvvm       *virtv1.VirtualMachine
			enriched   string
		)

		BeforeEach(func() {
			enrichedSpecs := commonnetwork.InterfaceSpecList{}
			for _, spec := range attachedSpecs {
				if spec.Type != string(v1alpha2.NetworksTypeMain) {
					spec.IPAssignmentMode = commonnetwork.IPAssignmentModeDHCP
					spec.IPAddressNames = []string{ipName}
				}
				enrichedSpecs = append(enrichedSpecs, spec)
			}
			var err error
			enriched, err = enrichedSpecs.ToString()
			Expect(err).NotTo(HaveOccurred())

			kvvm = newKVVM(
				virtv1.Interface{Name: commonnetwork.NameDefaultInterface},
				virtv1.Interface{Name: additionalIfaceName},
			)
			fakeClient, _, vmState = setupEnvironment(
				newVM(mainNetwork, additionalNetwork), kvvm, newKVVMI(), mac,
				newReadyClusterNetworkWithPool(cnName, poolName),
				newSDNIPAddressUnstructured(ipName, namespace, string(vmUID), "ClusterNetwork", cnName, "192.168.201.10", true),
				newPod(map[string]string{
					annotations.AnnNetworksSpec:               enriched,
					annotations.AnnTapProvisionByDVPSupported: "true",
				}),
				newNodeWithTapSupport(nodeName),
			)
		})

		It("reports no drift once the enriched spec is on the pod", func() {
			h := &SyncKvvmHandler{client: fakeClient}

			outOfSync, err := h.networksOutOfSync(ctx, vmState, kvvm)
			Expect(err).NotTo(HaveOccurred())
			Expect(outOfSync).To(BeFalse())
		})

		It("leaves the enriched annotation untouched", func() {
			h := &SyncKvvmHandler{client: fakeClient}

			_, err := h.patchPodNetworkAnnotation(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
			Expect(getPod(fakeClient).Annotations[annotations.AnnNetworksSpec]).To(Equal(enriched))
		})
	})

	Context("SDN reporting an error on the interface being detached", func() {
		// The entry of a detaching interface is kept in networks-spec until the device
		// leaves the domain, so SDN keeps reporting it. Its conditions must not gate the
		// KVVM update: that update is what requests the unplug in the first place, and
		// blocking it would keep the interface, and its veth, forever.
		newStatusAnnotation := func(name string) string {
			raw, err := json.Marshal([]commonnetwork.InterfaceStatus{{
				Type:   string(v1alpha2.NetworksTypeClusterNetwork),
				Name:   name,
				IfName: additionalIfaceName,
				Mac:    macAddr,
				Conditions: []metav1.Condition{{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "InterfaceNotConfigured",
					Message: "the network is gone",
				}},
			}})
			Expect(err).NotTo(HaveOccurred())
			return string(raw)
		}

		newEnv := func(networks ...v1alpha2.NetworksSpec) (client.WithWatch, state.VirtualMachineState) {
			attached, err := attachedSpecs.ToString()
			Expect(err).NotTo(HaveOccurred())
			fakeClient, _, vmState := setupEnvironment(
				newVM(networks...),
				newKVVM(
					virtv1.Interface{Name: commonnetwork.NameDefaultInterface},
					virtv1.Interface{Name: additionalIfaceName, State: virtv1.InterfaceStateAbsent},
				),
				newKVVMI(), mac, newReadyClusterNetwork(cnName),
				newPod(map[string]string{
					annotations.AnnNetworksSpec:               attached,
					annotations.AnnNetworksStatus:             newStatusAnnotation(cnName),
					annotations.AnnTapProvisionByDVPSupported: "true",
				}),
				newNodeWithTapSupport(nodeName),
			)
			return fakeClient, vmState
		}

		It("does not block the network readiness of a network the spec dropped", func() {
			fakeClient, vmState := newEnv(mainNetwork)
			h := &SyncKvvmHandler{client: fakeClient}

			ready, err := h.isNetworkReadyOnPod(ctx, vmState, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())
		})

		It("still blocks the network readiness of a network the spec asks for", func() {
			fakeClient, vmState := newEnv(mainNetwork, additionalNetwork)
			h := &SyncKvvmHandler{client: fakeClient}

			ready, err := h.isNetworkReadyOnPod(ctx, vmState, []string{cnName})
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})
	})
})

var _ = Describe("NetworkInterfaceHandler network status of a Main-only virtual machine", func() {
	const (
		vmName    = "vm-status-detach"
		namespace = "default"
		cnName    = "cnet-status-detach"
		macAddr   = "aa:bb:cc:dd:ee:03"
		ifaceName = "veth_cnstatusdetach"
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	// The last additional network has just been removed from the spec while its device is
	// still leaving the domain. Dropping the status entry right away makes the
	// VirtualMachineMACAddress unattached, and its address is handed back while the guest
	// still holds the interface.
	detachingStatus := v1alpha2.NetworksStatus{
		ID:   2,
		Type: v1alpha2.NetworksTypeClusterNetwork,
		Name: cnName,
		MAC:  macAddr,
	}

	newVM := func() *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: "vm-status-detach-uid"},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(commonnetwork.ReservedMainID)},
				},
			},
		}
		vm.Status.Networks = []v1alpha2.NetworksStatus{
			{ID: commonnetwork.ReservedMainID, Type: v1alpha2.NetworksTypeMain},
			detachingStatus,
		}
		return vm
	}

	newKVVMI := func(ifaces ...virtv1.Interface) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(vmName, namespace)
		kvvmi.Status.Phase = virtv1.Running
		kvvmi.Spec.Domain.Devices.Interfaces = ifaces
		return kvvmi
	}

	update := func(objs ...client.Object) []v1alpha2.NetworksStatus {
		_, _, vmState := setupEnvironment(newVM(), objs...)
		h := &NetworkInterfaceHandler{virtualMachineCIDRs: []string{"10.0.0.0/24"}}
		vm := vmState.VirtualMachine().Changed()
		_, err := h.UpdateNetworkStatus(ctx, vmState, vm)
		Expect(err).NotTo(HaveOccurred())
		return vm.Status.Networks
	}

	It("keeps the entry while the interface is still in the internal virtual machine instance", func() {
		networksStatus := update(newKVVMI(virtv1.Interface{Name: ifaceName, MacAddress: macAddr}))

		Expect(networksStatus).To(ContainElement(detachingStatus))
	})

	It("drops the entry once the interface has left the internal virtual machine instance", func() {
		networksStatus := update(newKVVMI())

		Expect(networksStatus).To(ConsistOf(v1alpha2.NetworksStatus{
			ID:   commonnetwork.ReservedMainID,
			Type: v1alpha2.NetworksTypeMain,
		}))
	})

	It("drops the entry when the virtual machine is not running", func() {
		networksStatus := update()

		Expect(networksStatus).To(ConsistOf(v1alpha2.NetworksStatus{
			ID:   commonnetwork.ReservedMainID,
			Type: v1alpha2.NetworksTypeMain,
		}))
	})
})

var _ = Describe("podNetworks resolving the annotation of a launcher pod", func() {
	ctx := testutil.ContextBackgroundWithNoOpLogger()

	const (
		wantedIface  = "veth_cnwanted"
		leavingIface = "veth_cnleaving"
		leavingEntry = `[{"id":3,"type":"ClusterNetwork","name":"cn-leaving","ifName":"` + leavingIface + `","uid":64535,"gid":64535}]`
		unparseable  = "{not a list at all"
	)

	desired := podNetworks{
		specs: commonnetwork.InterfaceSpecList{{
			ID: 2, Type: string(v1alpha2.NetworksTypeClusterNetwork),
			Name: "cn-wanted", InterfaceName: wantedIface, UID: 64535, GID: 64535,
		}},
		awaitingRemoval: map[string]struct{}{leavingIface: {}},
	}

	podWith := func(anns map[string]string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "d8v-vm-x", Annotations: anns}}
	}

	It("carries over the entry of an interface still leaving the machine", func() {
		value, leaveAlone, err := desired.annotationFor(ctx, podWith(map[string]string{
			annotations.AnnNetworksSpec: leavingEntry,
		}))

		Expect(err).NotTo(HaveOccurred())
		Expect(leaveAlone).To(BeFalse())
		Expect(value).To(ContainSubstring(wantedIface))
		Expect(value).To(ContainSubstring(leavingIface))
	})

	It("advertises the desired spec as is when the current annotation cannot be parsed", func() {
		value, leaveAlone, err := desired.annotationFor(ctx, podWith(map[string]string{
			annotations.AnnNetworksSpec: unparseable,
		}))

		Expect(err).NotTo(HaveOccurred())
		Expect(leaveAlone).To(BeFalse())
		Expect(value).To(ContainSubstring(wantedIface))
		Expect(value).NotTo(ContainSubstring(leavingIface))
	})

	It("leaves a pod alone when it was never annotated and there is nothing to advertise", func() {
		empty := podNetworks{}

		value, leaveAlone, err := empty.annotationFor(ctx, podWith(nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(Equal(emptyNetworksSpec))
		Expect(leaveAlone).To(BeTrue())
	})

	It("does not leave an annotated pod alone even when the spec is empty", func() {
		empty := podNetworks{}

		value, leaveAlone, err := empty.annotationFor(ctx, podWith(map[string]string{
			annotations.AnnNetworksSpec: leavingEntry,
		}))

		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(Equal(emptyNetworksSpec))
		Expect(leaveAlone).To(BeFalse())
	})
})
