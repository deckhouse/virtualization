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

package watcher

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("PCIDeviceWatcher", func() {
	Describe("requestsByPCIDevice", func() {
		It("returns a request for the owner NodePCIDevice", func() {
			pciDevice := &v1alpha2.PCIDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pci-device-1",
					Namespace: "test-ns",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: v1alpha2.SchemeGroupVersion.String(),
						Kind:       v1alpha2.NodePCIDeviceKind,
						Name:       "node-pci-device-1",
					}},
				},
			}

			requests := requestsByPCIDevice(pciDevice)

			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("node-pci-device-1"))
			Expect(requests[0].Namespace).To(BeEmpty())
		})

		It("falls back to the PCIDevice name", func() {
			pciDevice := &v1alpha2.PCIDevice{ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "test-ns"}}

			requests := requestsByPCIDevice(pciDevice)

			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("pci-device-1"))
		})
	})

	Describe("shouldProcessPCIDeviceUpdate", func() {
		var oldObj *v1alpha2.PCIDevice

		BeforeEach(func() {
			oldObj = &v1alpha2.PCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1"},
				Status: v1alpha2.PCIDeviceStatus{Conditions: []metav1.Condition{{
					Type:   "Attached",
					Status: metav1.ConditionFalse,
					Reason: "Available",
				}}},
			}
		})

		It("ignores unchanged objects", func() {
			sameObj := oldObj.DeepCopy()

			Expect(shouldProcessPCIDeviceUpdate(oldObj, sameObj)).To(BeFalse())
		})

		It("processes condition changes", func() {
			changedConditions := oldObj.DeepCopy()
			changedConditions.Status.Conditions[0].Status = metav1.ConditionTrue

			Expect(shouldProcessPCIDeviceUpdate(oldObj, changedConditions)).To(BeTrue())
		})

		It("processes owner reference changes", func() {
			changedOwners := oldObj.DeepCopy()
			changedOwners.OwnerReferences = []metav1.OwnerReference{{Name: "node-pci-device-1"}}

			Expect(shouldProcessPCIDeviceUpdate(oldObj, changedOwners)).To(BeTrue())
		})

		It("ignores nil objects", func() {
			Expect(shouldProcessPCIDeviceUpdate(nil, oldObj)).To(BeFalse())
			Expect(shouldProcessPCIDeviceUpdate(oldObj, nil)).To(BeFalse())
		})
	})
})
