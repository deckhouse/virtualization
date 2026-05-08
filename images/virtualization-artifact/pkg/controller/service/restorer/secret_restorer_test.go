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
})
