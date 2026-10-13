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

package restorer

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestRestorer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Restorer Suite")
}

var _ = Describe("SecretRestorer", func() {
	Describe("RestoreMACAddressOrder", func() {
		It("keeps an empty slot for the main network", func() {
			vm := &v1alpha2.VirtualMachine{
				Status: v1alpha2.VirtualMachineStatus{
					Networks: []v1alpha2.NetworksStatus{
						{Type: v1alpha2.NetworksTypeMain},
						{Type: v1alpha2.NetworksTypeNetwork, MAC: "02:00:00:00:00:11"},
						{Type: v1alpha2.NetworksTypeNetwork, MAC: "02:00:00:00:00:22"},
					},
				},
			}

			vmJSON, err := json.Marshal(vm)
			Expect(err).NotTo(HaveOccurred())

			secret := &corev1.Secret{
				Data: map[string][]byte{virtualMachineKey: vmJSON},
			}

			order, err := (SecretRestorer{}).RestoreMACAddressOrder(context.Background(), secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"", "02:00:00:00:00:11", "02:00:00:00:00:22"}))
		})
	})

	Describe("setVirtualMachineIPAddress", func() {
		It("returns nil and does not write to secret when VM has no IP address reference", func() {
			secret := &corev1.Secret{Data: map[string][]byte{}}
			vm := &v1alpha2.VirtualMachine{}

			err := (SecretRestorer{}).setVirtualMachineIPAddress(
				context.Background(), secret, vm, v1alpha2.KeepIPAddressAlways,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Data).NotTo(HaveKey(virtualMachineIPAddressKey))
		})
	})

	Describe("setMainIPAddress", func() {
		newVM := func(specIPAddressName string) *v1alpha2.VirtualMachine {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
				Status: v1alpha2.VirtualMachineStatus{Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeMain, IPAddressName: "vm-main", IPAddress: "10.66.10.14"},
				}},
			}
			if specIPAddressName != "" {
				vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: specIPAddressName}}
			}
			return vm
		}
		newIPAddress := func(ipaType string) *sdnapi.IPAddress {
			return &sdnapi.IPAddress{
				ObjectMeta: metav1.ObjectMeta{Name: "vm-main", Namespace: "ns"},
				Spec:       sdnapi.IPAddressSpec{Type: ipaType},
			}
		}

		DescribeTable("saves the IPAddress",
			func(keep v1alpha2.KeepIPAddress, ipaType, specIPAddressName string, saved bool) {
				cl, err := testutil.NewFakeClientWithObjects(newIPAddress(ipaType))
				Expect(err).NotTo(HaveOccurred())
				secret := &corev1.Secret{Data: map[string][]byte{}}

				Expect(SecretRestorer{client: cl}.setMainIPAddress(context.Background(), secret, newVM(specIPAddressName), keep)).To(Succeed())
				if saved {
					Expect(secret.Data).To(HaveKey(mainIPAddressKey))
				} else {
					Expect(secret.Data).NotTo(HaveKey(mainIPAddressKey))
				}
			},
			Entry("Always, Auto", v1alpha2.KeepIPAddressAlways, sdnapi.IPAddressTypeAuto, "", true),
			Entry("Never, Auto, not named in the spec", v1alpha2.KeepIPAddressNever, sdnapi.IPAddressTypeAuto, "", false),
			Entry("Never, Auto, named in the spec", v1alpha2.KeepIPAddressNever, sdnapi.IPAddressTypeAuto, "vm-main", true),
			Entry("Never, Static", v1alpha2.KeepIPAddressNever, sdnapi.IPAddressTypeStatic, "", true),
		)

		It("fails when the IPAddress of the status is gone", func() {
			cl, err := testutil.NewFakeClientWithObjects()
			Expect(err).NotTo(HaveOccurred())
			secret := &corev1.Secret{Data: map[string][]byte{}}

			err = SecretRestorer{client: cl}.setMainIPAddress(context.Background(), secret, newVM(""), v1alpha2.KeepIPAddressAlways)
			Expect(err).To(MatchError(ContainSubstring("was not found")))
			Expect(secret.Data).NotTo(HaveKey(mainIPAddressKey))
		})
	})
})
