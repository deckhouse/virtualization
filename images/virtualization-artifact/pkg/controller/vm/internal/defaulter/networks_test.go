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

package defaulter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/defaulter"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("NetworksDefaulter", func() {
	var (
		ctx               = testutil.ContextBackgroundWithNoOpLogger()
		networksDefaulter *defaulter.NetworksDefaulter
	)

	BeforeEach(func() {
		networksDefaulter = defaulter.NewNetworksDefaulter()
	})

	Describe("Default network IDs", func() {
		It("should assign id=1 to Main network when id=0", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeMain, ID: 0},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(1))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeMain))
			Expect(vm.Spec.Networks[0].ID).To(Equal(1))
		})

		It("should not change Main network id when it is already set to 1", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeMain, ID: 1},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(1))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeMain))
			Expect(vm.Spec.Networks[0].ID).To(Equal(1))
		})

		It("should assign sequential ids starting from 2 to networks with id=0", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeMain, ID: 0},
						{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network-1", ID: 0},
						{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network-2", ID: 0},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(3))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeMain))
			Expect(vm.Spec.Networks[0].ID).To(Equal(1))
			Expect(vm.Spec.Networks[1].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
			Expect(vm.Spec.Networks[1].Name).To(Equal("test-network-1"))
			Expect(vm.Spec.Networks[1].ID).To(Equal(2))
			Expect(vm.Spec.Networks[2].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
			Expect(vm.Spec.Networks[2].Name).To(Equal("test-network-2"))
			Expect(vm.Spec.Networks[2].ID).To(Equal(3))
		})

		It("should not change network id when it is already set", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeMain, ID: 1},
						{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network", ID: 5},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(2))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeMain))
			Expect(vm.Spec.Networks[0].ID).To(Equal(1))
			Expect(vm.Spec.Networks[1].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
			Expect(vm.Spec.Networks[1].ID).To(Equal(5))
		})

		It("should assign sequential ids considering already set ids", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeMain, ID: 0},
						{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network-1", ID: 5},
						{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network-2", ID: 0},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(3))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeMain))
			Expect(vm.Spec.Networks[0].ID).To(Equal(1))
			Expect(vm.Spec.Networks[1].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
			Expect(vm.Spec.Networks[1].Name).To(Equal("test-network-1"))
			Expect(vm.Spec.Networks[1].ID).To(Equal(5))
			Expect(vm.Spec.Networks[2].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
			Expect(vm.Spec.Networks[2].Name).To(Equal("test-network-2"))
			Expect(vm.Spec.Networks[2].ID).To(Equal(2))
		})

		It("should handle ClusterNetwork type correctly", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeMain, ID: 0},
						{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "test-cluster-network", ID: 0},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(2))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeMain))
			Expect(vm.Spec.Networks[0].ID).To(Equal(1))
			Expect(vm.Spec.Networks[1].Type).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
			Expect(vm.Spec.Networks[1].Name).To(Equal("test-cluster-network"))
			Expect(vm.Spec.Networks[1].ID).To(Equal(2))
		})

		It("should skip id=1 when assigning to non-Main networks", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeMain, ID: 1},
						{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network", ID: 0},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(2))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeMain))
			Expect(vm.Spec.Networks[0].ID).To(Equal(1))
			Expect(vm.Spec.Networks[1].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
			Expect(vm.Spec.Networks[1].ID).To(Equal(2))
		})

		It("should assign sequential ids starting from 2 when there is no Main network", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network-1", ID: 0},
						{Type: v1alpha2.NetworksTypeNetwork, Name: "test-network-2", ID: 0},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(2))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
			Expect(vm.Spec.Networks[0].Name).To(Equal("test-network-1"))
			Expect(vm.Spec.Networks[0].ID).To(Equal(2))
			Expect(vm.Spec.Networks[1].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
			Expect(vm.Spec.Networks[1].Name).To(Equal("test-network-2"))
			Expect(vm.Spec.Networks[1].ID).To(Equal(3))
		})

		It("should handle only ClusterNetwork without Main network", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: []v1alpha2.NetworksSpec{
						{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "test-cluster-network", ID: 0},
					},
				},
			}
			err := networksDefaulter.Default(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.Networks).To(HaveLen(1))
			Expect(vm.Spec.Networks[0].Type).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
			Expect(vm.Spec.Networks[0].Name).To(Equal("test-cluster-network"))
			Expect(vm.Spec.Networks[0].ID).To(Equal(2))
		})
	})
})
