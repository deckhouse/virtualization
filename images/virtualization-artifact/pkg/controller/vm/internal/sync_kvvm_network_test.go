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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

var _ = Describe("SyncKvvmHandler network sync across migration pods", func() {
	const (
		vmName             = "vm-mig"
		namespace          = "default"
		clusterNetworkName = "cnet-eno2"
		macName            = "vmmac-mig"
		macAddr            = "aa:bb:cc:dd:ee:ff"

		sourceNode              = "node-src"
		targetNode              = "node-dst"
		sourcePodName           = "d8v-src"
		targetPodName           = "d8v-dst"
		sourcePodUID  types.UID = "src-uid"
		targetPodUID  types.UID = "dst-uid"
	)

	var (
		ctx         = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient  client.WithWatch
		vmState     state.VirtualMachineState
		kvvm        *virtv1.VirtualMachine
		desiredSpec string
	)

	newReadyClusterNetwork := func(name string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(commonnetwork.ClusterNetworkGVK)
		u.SetName(name)
		Expect(unstructured.SetNestedSlice(u.Object, []any{
			map[string]any{"type": "Ready", "status": "True"},
		}, "status", "conditions")).To(Succeed())
		return u
	}

	newLauncherPod := func(podName string, uid types.UID, node, networksSpec string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        podName,
				Namespace:   namespace,
				UID:         uid,
				Labels:      map[string]string{virtv1.VirtualMachineNameLabel: vmName},
				Annotations: map[string]string{annotations.AnnNetworksSpec: networksSpec},
			},
			Spec:   corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}

	BeforeEach(func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: "vm-uid"},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(commonnetwork.ReservedMainID)},
					{Type: v1alpha2.NetworksTypeClusterNetwork, Name: clusterNetworkName, ID: ptr.To(2), VirtualMachineMACAddressName: macName},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeMain, ID: commonnetwork.ReservedMainID},
					{Type: v1alpha2.NetworksTypeClusterNetwork, Name: clusterNetworkName, ID: 2, MAC: macAddr, VirtualMachineMACAddressName: macName},
				},
			},
		}

		mac := &v1alpha2.VirtualMachineMACAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      macName,
				Namespace: namespace,
				Labels:    map[string]string{annotations.LabelVirtualMachineUID: string(vm.UID)},
			},
			Status: v1alpha2.VirtualMachineMACAddressStatus{
				Address: macAddr,
				Phase:   v1alpha2.VirtualMachineMACAddressPhaseAttached,
			},
		}

		kvvmi := newEmptyKVVMI(vmName, namespace)
		// During migration the KVVMI still points at the source node while both the
		// source and the target pod are active.
		kvvmi.Status.NodeName = sourceNode
		kvvmi.Status.ActivePods = map[types.UID]string{
			sourcePodUID: sourceNode,
			targetPodUID: targetNode,
		}

		desiredSpecList := commonnetwork.CreateNetworkSpec(vm, []*v1alpha2.VirtualMachineMACAddress{mac})
		var err error
		desiredSpec, err = desiredSpecList.ToString()
		Expect(err).NotTo(HaveOccurred())
		Expect(desiredSpec).NotTo(Equal("[]"))

		// KVVM template already carries the right interfaces, so only the pod annotation
		// is out of sync.
		kvvm = newEmptyKVVM(vmName, namespace)
		kvvm.Spec.Template = &virtv1.VirtualMachineInstanceTemplateSpec{}
		for _, spec := range desiredSpecList {
			kvvm.Spec.Template.Spec.Domain.Devices.Interfaces = append(
				kvvm.Spec.Template.Spec.Domain.Devices.Interfaces,
				virtv1.Interface{Name: spec.InterfaceName},
			)
		}

		// The source pod is already configured; the migration target inherited "[]".
		sourcePod := newLauncherPod(sourcePodName, sourcePodUID, sourceNode, desiredSpec)
		sourcePod.Annotations[annotations.AnnTapProvisionByDVPSupported] = "true"
		targetPod := newLauncherPod(targetPodName, targetPodUID, targetNode, "[]")

		fakeClient, _, vmState = setupEnvironment(vm, kvvm, kvvmi, mac, newReadyClusterNetwork(clusterNetworkName), sourcePod, targetPod)
	})

	It("reports out of sync while the migration target pod carries a stale annotation", func() {
		h := &SyncKvvmHandler{client: fakeClient}

		outOfSync, err := h.networksOutOfSync(ctx, vmState, kvvm)
		Expect(err).NotTo(HaveOccurred())
		Expect(outOfSync).To(BeTrue())
	})

	It("patches the desired annotation onto the migration target pod", func() {
		h := &SyncKvvmHandler{client: fakeClient}

		desired, err := h.patchPodNetworkAnnotation(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(desired).To(ConsistOf(clusterNetworkName))

		target := &corev1.Pod{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: targetPodName, Namespace: namespace}, target)).To(Succeed())
		Expect(target.Annotations[annotations.AnnNetworksSpec]).To(Equal(desiredSpec))
		Expect(target.Annotations[annotations.AnnTapProvisionByDVPSupported]).To(Equal("true"))

		source := &corev1.Pod{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: sourcePodName, Namespace: namespace}, source)).To(Succeed())
		Expect(source.Annotations[annotations.AnnNetworksSpec]).To(Equal(desiredSpec))
	})

	It("reports in sync once both pods carry the desired annotation", func() {
		h := &SyncKvvmHandler{client: fakeClient}

		_, err := h.patchPodNetworkAnnotation(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		outOfSync, err := h.networksOutOfSync(ctx, vmState, kvvm)
		Expect(err).NotTo(HaveOccurred())
		Expect(outOfSync).To(BeFalse())
	})
})
