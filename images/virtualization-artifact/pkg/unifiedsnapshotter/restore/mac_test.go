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

package restore

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// vmmacObj builds a captured VirtualMachineMACAddress: specAddress is what the user asked for (empty
// when the address was auto-allocated), statusAddress is what was actually allocated.
func vmmacObj(name, specAddress, statusAddress string) unstructured.Unstructured {
	o := obj(v1alpha2.VirtualMachineMACAddressKind, name)
	o.Object["spec"] = map[string]interface{}{"address": specAddress}
	o.Object["status"] = map[string]interface{}{"address": statusAddress}
	return o
}

// vmWithNetworks builds a captured VirtualMachine whose spec.networks and status.networks agree by
// index, the way the virtual machine controller writes them (Main first).
func vmWithNetworks(specLinks, statusLinks []string) unstructured.Unstructured {
	o := obj(v1alpha2.VirtualMachineKind, "vm")

	specNetworks := make([]interface{}, 0, len(specLinks))
	for i, link := range specLinks {
		network := map[string]interface{}{"type": v1alpha2.NetworksTypeClusterNetwork}
		if i == 0 {
			network["type"] = v1alpha2.NetworksTypeMain
		}
		if link != "" {
			network["virtualMachineMACAddressName"] = link
		}
		specNetworks = append(specNetworks, network)
	}
	o.Object["spec"] = map[string]interface{}{"networks": specNetworks}

	statusNetworks := make([]interface{}, 0, len(statusLinks))
	for _, link := range statusLinks {
		network := map[string]interface{}{"type": v1alpha2.NetworksTypeClusterNetwork}
		if link != "" {
			network["virtualMachineMACAddressName"] = link
		}
		statusNetworks = append(statusNetworks, network)
	}
	o.Object["status"] = map[string]interface{}{"networks": statusNetworks}

	return o
}

func macLinks(t *testing.T, vm unstructured.Unstructured) []string {
	t.Helper()
	networks, _, err := unstructured.NestedSlice(vm.Object, "spec", "networks")
	if err != nil {
		t.Fatalf("read spec.networks: %v", err)
	}
	links := make([]string, 0, len(networks))
	for _, n := range networks {
		name, _ := n.(map[string]interface{})["virtualMachineMACAddressName"].(string)
		links = append(links, name)
	}
	return links
}

func specAddress(t *testing.T, o unstructured.Unstructured) string {
	t.Helper()
	address, _, err := unstructured.NestedString(o.Object, "spec", "address")
	if err != nil {
		t.Fatalf("read spec.address: %v", err)
	}
	return address
}

// An auto-allocated MAC lives only in status: without pinning it into spec, the restored machine comes
// back with a freshly generated address.
func TestApplyCapturedMACAddresses_PinsAllocatedAddresses(t *testing.T) {
	objs := []unstructured.Unstructured{
		vmmacObj("vm-auto-1", "", "be:5e:2b:ae:c7:b3"),
		vmmacObj("vm-auto-2", "", "be:5e:2b:c4:ab:d1"),
	}

	if err := applyCapturedMACAddresses(objs); err != nil {
		t.Fatalf("applyCapturedMACAddresses: %v", err)
	}

	if got := specAddress(t, objs[0]); got != "be:5e:2b:ae:c7:b3" {
		t.Errorf("spec.address = %q, want the allocated be:5e:2b:ae:c7:b3", got)
	}
	if got := specAddress(t, objs[1]); got != "be:5e:2b:c4:ab:d1" {
		t.Errorf("spec.address = %q, want the allocated be:5e:2b:c4:ab:d1", got)
	}
}

// An address the user asked for explicitly is already in spec and must survive verbatim.
func TestApplyCapturedMACAddresses_KeepsAnExplicitAddress(t *testing.T) {
	objs := []unstructured.Unstructured{vmmacObj("vm-static", "be:5e:2b:00:00:01", "be:5e:2b:00:00:01")}

	if err := applyCapturedMACAddresses(objs); err != nil {
		t.Fatalf("applyCapturedMACAddresses: %v", err)
	}
	if got := specAddress(t, objs[0]); got != "be:5e:2b:00:00:01" {
		t.Errorf("spec.address = %q, want be:5e:2b:00:00:01", got)
	}
}

func TestApplyCapturedMACAddresses_FailsClosedWithNoAddressAtAll(t *testing.T) {
	objs := []unstructured.Unstructured{vmmacObj("vm-unbound", "", "")}

	if err := applyCapturedMACAddresses(objs); err == nil {
		t.Fatal("expected an error: neither spec nor status carries an address to re-claim")
	}
}

// A machine whose MACs were auto-allocated names nothing in spec — only status knows which object backs
// which interface, so the link has to be rebuilt from there.
func TestApplyCapturedMACAddresses_RelinksInterfacesWithNoNameInSpec(t *testing.T) {
	objs := []unstructured.Unstructured{
		vmWithNetworks(
			[]string{"", "", ""},
			[]string{"", "vm-auto-1", "vm-auto-2"},
		),
		vmmacObj("vm-auto-1", "", "be:5e:2b:ae:c7:b3"),
		vmmacObj("vm-auto-2", "", "be:5e:2b:c4:ab:d1"),
	}

	if err := applyCapturedMACAddresses(objs); err != nil {
		t.Fatalf("applyCapturedMACAddresses: %v", err)
	}

	got := macLinks(t, objs[0])
	want := []string{"", "vm-auto-1", "vm-auto-2"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("spec.networks[%d].virtualMachineMACAddressName = %q, want %q", i, got[i], want[i])
		}
	}
}

// A link to something the restore will not create would leave the machine waiting for an object that
// never appears; unlinking lets it allocate a fresh address instead.
func TestApplyCapturedMACAddresses_UnlinksWhatWasNotCaptured(t *testing.T) {
	objs := []unstructured.Unstructured{
		vmWithNetworks(
			[]string{"", "vm-gone", "vm-auto-2"},
			[]string{"", "vm-gone", "vm-auto-2"},
		),
		vmmacObj("vm-auto-2", "", "be:5e:2b:c4:ab:d1"),
	}

	if err := applyCapturedMACAddresses(objs); err != nil {
		t.Fatalf("applyCapturedMACAddresses: %v", err)
	}

	got := macLinks(t, objs[0])
	want := []string{"", "", "vm-auto-2"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("spec.networks[%d].virtualMachineMACAddressName = %q, want %q", i, got[i], want[i])
		}
	}
}

// Nothing captured at all is not an error: every link is cleared and the restored machine allocates
// fresh addresses, matching what the Secret-based restore path does.
func TestApplyCapturedMACAddresses_ClearsEveryLinkWhenNothingWasCaptured(t *testing.T) {
	objs := []unstructured.Unstructured{
		vmWithNetworks([]string{"", "vm-auto-1"}, nil),
	}

	if err := applyCapturedMACAddresses(objs); err != nil {
		t.Fatalf("applyCapturedMACAddresses: %v", err)
	}

	for i, got := range macLinks(t, objs[0]) {
		if got != "" {
			t.Errorf("spec.networks[%d].virtualMachineMACAddressName = %q, want it cleared", i, got)
		}
	}
}

// A status shorter than spec leaves the interface order unknowable, and guessing it would hand an
// interface someone else's address.
func TestApplyCapturedMACAddresses_RefusesAMisalignedStatus(t *testing.T) {
	objs := []unstructured.Unstructured{
		vmWithNetworks([]string{"", "", ""}, []string{"", "vm-auto-1"}),
		vmmacObj("vm-auto-1", "", "be:5e:2b:ae:c7:b3"),
	}

	if err := applyCapturedMACAddresses(objs); err == nil {
		t.Fatal("expected an error: spec declares more networks than status can order")
	}
}

func TestApplyCapturedMACAddresses_IgnoresAMachineWithoutNetworks(t *testing.T) {
	objs := []unstructured.Unstructured{obj(v1alpha2.VirtualMachineKind, "vm")}

	if err := applyCapturedMACAddresses(objs); err != nil {
		t.Fatalf("applyCapturedMACAddresses: %v", err)
	}
}

// The allocated address and the interface order both live in status, which sanitization strips — so
// this only works if the MAC pass runs first. Compiling the whole node is what pins that ordering.
func TestCompileVirtualMachineSnapshot_KeepsTheCapturedMACAddresses(t *testing.T) {
	manifests := &fakeManifests{byContent: map[string][]unstructured.Unstructured{
		vmsContent: {
			vmWithNetworks([]string{"", ""}, []string{"", "vm-auto-1"}),
			vmmacObj("vm-auto-1", "", "be:5e:2b:ae:c7:b3"),
		},
	}}
	c := newCompiler(t, manifests, vmSnapshot())

	objs, err := c.CompileVirtualMachineSnapshot(context.Background(), testNamespace, "vms")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	var vm, vmmac *unstructured.Unstructured
	for i := range objs {
		switch objs[i].GetKind() {
		case v1alpha2.VirtualMachineKind:
			vm = &objs[i]
		case v1alpha2.VirtualMachineMACAddressKind:
			vmmac = &objs[i]
		}
	}
	if vm == nil || vmmac == nil {
		t.Fatal("compiled output is missing the virtual machine or its MAC address")
	}

	if got := specAddress(t, *vmmac); got != "be:5e:2b:ae:c7:b3" {
		t.Errorf("spec.address = %q, want the captured be:5e:2b:ae:c7:b3", got)
	}
	if _, found, _ := unstructured.NestedMap(vmmac.Object, "status"); found {
		t.Error("status survived into the restore output")
	}
	if got := macLinks(t, *vm); got[1] != "vm-auto-1" {
		t.Errorf("spec.networks[1].virtualMachineMACAddressName = %q, want vm-auto-1", got[1])
	}
}
