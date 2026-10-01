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
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("NetworkInterfaceHandler MAC addresses in the network status", func() {
	const (
		vmName    = "vm-status-mac"
		namespace = "default"
		vmUID     = "vm-status-mac-uid"
		cnA       = "cn-a"
		cnB       = "cn-b"
		cnC       = "cn-c"
		// cn-a holds the greater address on purpose: a pool handing out free addresses
		// in order would swap the two networks.
		macOfA = "aa:bb:cc:dd:ee:02"
		macOfB = "aa:bb:cc:dd:ee:01"
		macOfC = "aa:bb:cc:dd:ee:03"
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	newVMMAC := func(name, address string) *v1alpha2.VirtualMachineMACAddress {
		return &v1alpha2.VirtualMachineMACAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    map[string]string{annotations.LabelVirtualMachineUID: vmUID},
			},
			Status: v1alpha2.VirtualMachineMACAddressStatus{
				Address:        address,
				VirtualMachine: vmName,
				Phase:          v1alpha2.VirtualMachineMACAddressPhaseAttached,
			},
		}
	}

	newVM := func(networks ...string) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: vmUID},
		}
		for i, name := range networks {
			vm.Spec.Networks = append(vm.Spec.Networks, v1alpha2.NetworksSpec{
				ID: ptr.To(i + 2), Type: v1alpha2.NetworksTypeClusterNetwork, Name: name,
			})
		}
		vm.Status.Networks = []v1alpha2.NetworksStatus{
			{ID: 2, Type: v1alpha2.NetworksTypeClusterNetwork, Name: cnA, MAC: macOfA},
			{ID: 3, Type: v1alpha2.NetworksTypeClusterNetwork, Name: cnB, MAC: macOfB},
		}
		return vm
	}

	unschedulableKVVM := func() *virtv1.VirtualMachine {
		kvvm := newEmptyKVVM(vmName, namespace)
		kvvm.Status.PrintableStatus = virtv1.VirtualMachineStatusUnschedulable
		return kvvm
	}

	allVMMACs := func() []*v1alpha2.VirtualMachineMACAddress {
		return []*v1alpha2.VirtualMachineMACAddress{newVMMAC("mac-a", macOfA), newVMMAC("mac-b", macOfB), newVMMAC("mac-c", macOfC)}
	}

	updateWith := func(vm *v1alpha2.VirtualMachine, vmmacs []*v1alpha2.VirtualMachineMACAddress, objs ...client.Object) map[string]string {
		for _, vmmac := range vmmacs {
			objs = append(objs, vmmac)
		}
		_, _, vmState := setupEnvironment(vm, objs...)
		h := &NetworkInterfaceHandler{}
		changed := vmState.VirtualMachine().Changed()
		_, err := h.UpdateNetworkStatus(ctx, vmState, changed)
		Expect(err).NotTo(HaveOccurred())

		macs := make(map[string]string)
		for _, ns := range changed.Status.Networks {
			macs[ns.Name] = ns.MAC
		}
		return macs
	}

	update := func(vm *v1alpha2.VirtualMachine, objs ...client.Object) map[string]string {
		return updateWith(vm, allVMMACs(), objs...)
	}

	It("keeps the MAC addresses while the internal virtual machine is unschedulable", func() {
		Expect(update(newVM(cnA, cnB), unschedulableKVVM())).To(Equal(map[string]string{cnA: macOfA, cnB: macOfB}))
	})

	It("drops the MAC addresses when there is no internal virtual machine", func() {
		Expect(update(newVM(cnA, cnB))).To(Equal(map[string]string{cnA: "", cnB: ""}))
	})

	It("reports no MAC address for a network the machine has never been given one", func() {
		Expect(update(newVM(cnA, cnB, cnC), unschedulableKVVM())).To(Equal(map[string]string{cnA: macOfA, cnB: macOfB, cnC: ""}))
	})

	It("drops a kept MAC address no VirtualMachineMACAddress leases any more", func() {
		Expect(updateWith(newVM(cnA, cnB), []*v1alpha2.VirtualMachineMACAddress{newVMMAC("mac-b", macOfB)}, unschedulableKVVM())).
			To(Equal(map[string]string{cnA: "", cnB: macOfB}))
	})

	// The KVVM template renders only the Ready networks: cn-a is not Ready yet.
	It("does not report the interface of another network when a network is not rendered", func() {
		vm := newVM(cnA, cnB)
		vm.Status.Networks = nil
		vmmacs := allVMMACs()[:2]

		kvvm := newEmptyKVVM(vmName, namespace)
		kvvm.Status.PrintableStatus = virtv1.VirtualMachineStatusRunning
		kvvm.Spec.Template = &virtv1.VirtualMachineInstanceTemplateSpec{}
		var cnBMAC string
		for _, spec := range commonnetwork.CreateNetworkSpec(vm, vm.Spec.Networks[1:], vmmacs, true) {
			kvvm.Spec.Template.Spec.Domain.Devices.Interfaces = append(kvvm.Spec.Template.Spec.Domain.Devices.Interfaces,
				virtv1.Interface{Name: spec.InterfaceName, MacAddress: spec.MAC})
			cnBMAC = spec.MAC
		}

		Expect(updateWith(vm, vmmacs, kvvm)).To(Equal(map[string]string{cnA: "", cnB: cnBMAC}))
	})

	// A restore deletes the KVVM for its maintenance and pins the captured addresses in the spec
	// while the status still holds the addresses the machine had before.
	It("binds the networks to the addresses a restore pins while there is no internal virtual machine", func() {
		vm := newVM(cnA, cnB)
		vm.Spec.Networks[0].VirtualMachineMACAddressName = "mac-c"

		_, _, vmState := setupEnvironment(vm, newVMMAC("mac-a", macOfA), newVMMAC("mac-b", macOfB), newVMMAC("mac-c", macOfC))
		changed := vmState.VirtualMachine().Changed()
		_, err := (&NetworkInterfaceHandler{}).UpdateNetworkStatus(ctx, vmState, changed)
		Expect(err).NotTo(HaveOccurred())

		Expect(changed.Status.Networks).To(ContainElement(And(
			HaveField("Name", cnA), HaveField("MAC", ""), HaveField("VirtualMachineMACAddressName", "mac-c"),
		)))
	})

	It("renders the addresses a restore pins into a new internal virtual machine", func() {
		vm := newVM(cnA, cnB)
		vm.Spec.Networks[0].VirtualMachineMACAddressName = "mac-c"

		specs := commonnetwork.CreateNetworkSpec(vm, vm.Spec.Networks, allVMMACs(), false)

		Expect(specs).To(ContainElement(And(HaveField("Name", cnA), HaveField("MAC", macOfC))))
	})

	It("keeps the running address of a network whose pin is changed", func() {
		vm := newVM(cnA, cnB)
		vm.Spec.Networks[0].VirtualMachineMACAddressName = "mac-c"
		kvvm := newEmptyKVVM(vmName, namespace)
		kvvm.Status.PrintableStatus = virtv1.VirtualMachineStatusRunning
		kvvm.Spec.Template = &virtv1.VirtualMachineInstanceTemplateSpec{}
		for _, spec := range commonnetwork.CreateNetworkSpec(vm, vm.Spec.Networks, allVMMACs(), true) {
			kvvm.Spec.Template.Spec.Domain.Devices.Interfaces = append(kvvm.Spec.Template.Spec.Domain.Devices.Interfaces,
				virtv1.Interface{Name: spec.InterfaceName, MacAddress: spec.MAC})
		}

		Expect(update(vm, kvvm)).To(Equal(map[string]string{cnA: macOfA, cnB: macOfB}))
	})
})
