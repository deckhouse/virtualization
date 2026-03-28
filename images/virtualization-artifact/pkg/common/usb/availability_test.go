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

package usb

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("availability helpers", func() {
	newNode := func(name string, totalPorts, usedHSPorts, usedSSPorts string) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{
			annotations.AnnUSBIPTotalPorts:             totalPorts,
			annotations.AnnUSBIPHighSpeedHubUsedPorts:  usedHSPorts,
			annotations.AnnUSBIPSuperSpeedHubUsedPorts: usedSSPorts,
		}}}
	}

	newVM := func(name, nodeName string, statuses ...v1alpha2.USBDeviceStatusRef) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Status: v1alpha2.VirtualMachineStatus{
				Node:       nodeName,
				USBDevices: statuses,
			},
		}
	}

	newUSBDevice := func(name, nodeName string, speed int) *v1alpha2.USBDevice {
		return &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Status: v1alpha2.USBDeviceStatus{
				NodeName: nodeName,
				Attributes: v1alpha2.NodeUSBDeviceAttributes{
					Speed: speed,
				},
			},
		}
	}

	newClient := func(objects ...client.Object) client.Client {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objects...).
			WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).
			Build()
	}

	It("excludes local attached USB devices of the same speed class from used port accounting", func() {
		cl := newClient(
			newNode("node-1", "2", "1", "0"),
			newVM("vm-1", "node-1", v1alpha2.USBDeviceStatusRef{Name: "usb-local", Attached: true}),
			newUSBDevice("usb-local", "node-1", 480),
		)

		hasFree, err := CheckFreePortForRequestOnNodeExcludingLocalUSBs(context.Background(), cl, "node-1", 480, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFree).To(BeTrue())
	})

	It("does not exclude local attached USB devices from another speed class", func() {
		cl := newClient(
			newNode("node-1", "2", "1", "0"),
			newVM("vm-1", "node-1", v1alpha2.USBDeviceStatusRef{Name: "usb-local-ss", Attached: true}),
			newUSBDevice("usb-local-ss", "node-1", 5000),
		)

		hasFree, err := CheckFreePortForRequestOnNodeExcludingLocalUSBs(context.Background(), cl, "node-1", 480, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFree).To(BeFalse())
	})

	It("ignores stale VM status entries when the referenced USBDevice is missing", func() {
		cl := newClient(
			newNode("node-1", "2", "1", "0"),
			newVM("vm-1", "node-1", v1alpha2.USBDeviceStatusRef{Name: "missing-usb", Attached: true}),
		)

		hasFree, err := CheckFreePortForRequestOnNodeExcludingLocalUSBs(context.Background(), cl, "node-1", 480, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFree).To(BeFalse())
	})

	It("clamps effective used ports to zero when excluded local devices exceed node annotations", func() {
		cl := newClient(
			newNode("node-1", "2", "0", "0"),
			newVM(
				"vm-1",
				"node-1",
				v1alpha2.USBDeviceStatusRef{Name: "usb-local-1", Attached: true},
				v1alpha2.USBDeviceStatusRef{Name: "usb-local-2", Attached: true},
			),
			newUSBDevice("usb-local-1", "node-1", 480),
			newUSBDevice("usb-local-2", "node-1", 480),
		)

		hasFree, err := CheckFreePortForRequestOnNodeExcludingLocalUSBs(context.Background(), cl, "node-1", 480, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasFree).To(BeTrue())
	})
})
