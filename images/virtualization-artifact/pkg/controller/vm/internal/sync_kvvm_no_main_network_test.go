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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("filterReadyNetworks", func() {
	const (
		vmName      = "vm"
		namespace   = "vms"
		networkName = "not-ready-net"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
	)

	newNotReadyNetwork := func() *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(commonnetwork.NetworkGVK)
		u.SetName(networkName)
		u.SetNamespace(namespace)
		Expect(unstructured.SetNestedSlice(u.Object, []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "False",
			},
		}, "status", "conditions")).To(Succeed())
		return u
	}

	newVM := func(networks ...v1alpha2.NetworksSpec) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: "vm-uid"},
			Spec:       v1alpha2.VirtualMachineSpec{Networks: networks},
		}
	}

	BeforeEach(func() {
		var err error
		fakeClient, err = testutil.NewFakeClientWithObjects(newNotReadyNetwork())
		Expect(err).NotTo(HaveOccurred())
	})

	It("keeps the Main network of a VM that lists no network at all", func() {
		kept, err := filterReadyNetworks(ctx, fakeClient, newVM())
		Expect(err).NotTo(HaveOccurred())
		Expect(kept).To(HaveLen(1))
		Expect(kept[0].Type).To(Equal(v1alpha2.NetworksTypeMain))

		Expect(commonnetwork.CreateNetworkSpec(newVM(), kept, nil)).To(HaveLen(1))
	})

	It("keeps the Main network while dropping a network that is not Ready", func() {
		vm := newVM(
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(commonnetwork.ReservedMainID)},
			v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: networkName, ID: ptr.To(2)},
		)

		kept, err := filterReadyNetworks(ctx, fakeClient, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(kept).To(HaveLen(1))
		Expect(kept[0].Type).To(Equal(v1alpha2.NetworksTypeMain))
	})

	It("asks for no interface at all when the only network of a VM without Main is not Ready", func() {
		vm := newVM(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: networkName, ID: ptr.To(2)})

		kept, err := filterReadyNetworks(ctx, fakeClient, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(kept).To(BeEmpty())

		Expect(commonnetwork.CreateNetworkSpec(vm, kept, nil)).To(BeEmpty())
	})
})
