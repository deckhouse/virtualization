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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func vmipObj(name, specType, address string) unstructured.Unstructured {
	o := obj(v1alpha2.VirtualMachineIPAddressKind, name)
	o.Object["spec"] = map[string]interface{}{"type": specType}
	o.Object["status"] = map[string]interface{}{"address": address}
	return o
}

func TestApplyKeepIPAddress(t *testing.T) {
	tests := []struct {
		name       string
		keep       v1alpha2.KeepIPAddress
		obj        unstructured.Unstructured
		wantType   string
		wantStatic string
		wantErr    bool
	}{
		{
			name:       "Always pins an allocated Auto address",
			keep:       v1alpha2.KeepIPAddressAlways,
			obj:        vmipObj("vmip", string(v1alpha2.VirtualMachineIPAddressTypeAuto), "10.66.10.14"),
			wantType:   string(v1alpha2.VirtualMachineIPAddressTypeStatic),
			wantStatic: "10.66.10.14",
		},
		{
			name:       "Always leaves an already Static address alone",
			keep:       v1alpha2.KeepIPAddressAlways,
			obj:        vmipObj("vmip", string(v1alpha2.VirtualMachineIPAddressTypeStatic), "10.66.10.14"),
			wantType:   string(v1alpha2.VirtualMachineIPAddressTypeStatic),
			wantStatic: "",
		},
		{
			name:     "Never keeps Auto, so a fresh address is allocated on restore",
			keep:     v1alpha2.KeepIPAddressNever,
			obj:      vmipObj("vmip", string(v1alpha2.VirtualMachineIPAddressTypeAuto), "10.66.10.14"),
			wantType: string(v1alpha2.VirtualMachineIPAddressTypeAuto),
		},
		{
			name:    "Always with no allocated address fails closed",
			keep:    v1alpha2.KeepIPAddressAlways,
			obj:     vmipObj("vmip", string(v1alpha2.VirtualMachineIPAddressTypeAuto), ""),
			wantErr: true,
		},
		{
			name:     "a non-VirtualMachineIPAddress object is untouched",
			keep:     v1alpha2.KeepIPAddressAlways,
			obj:      vmipObjOfKind(v1alpha2.VirtualMachineKind),
			wantType: string(v1alpha2.VirtualMachineIPAddressTypeAuto),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := tt.obj
			err := applyKeepIPAddress(&o, tt.keep)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			gotType, _, _ := unstructured.NestedString(o.Object, "spec", "type")
			if gotType != tt.wantType {
				t.Errorf("spec.type = %q, want %q", gotType, tt.wantType)
			}
			gotStatic, _, _ := unstructured.NestedString(o.Object, "spec", "staticIP")
			if gotStatic != tt.wantStatic {
				t.Errorf("spec.staticIP = %q, want %q", gotStatic, tt.wantStatic)
			}
		})
	}
}

func vmipObjOfKind(kind string) unstructured.Unstructured {
	o := obj(kind, "not-an-ip")
	o.Object["spec"] = map[string]interface{}{"type": string(v1alpha2.VirtualMachineIPAddressTypeAuto)}
	o.Object["status"] = map[string]interface{}{"address": "10.66.10.14"}
	return o
}

// The address lives in status, which sanitization strips — so the conversion only works if it runs
// first. Compiling the whole node is what pins that ordering.
func TestCompileVirtualMachineSnapshot_PinsTheCapturedIPAddress(t *testing.T) {
	manifests := &fakeManifests{byContent: map[string][]unstructured.Unstructured{
		vmsContent: {
			obj(v1alpha2.VirtualMachineKind, "vm"),
			vmipObj("vmip", string(v1alpha2.VirtualMachineIPAddressTypeAuto), "10.66.10.14"),
		},
	}}
	vms := vmSnapshot()
	vms.Spec.KeepIPAddress = v1alpha2.KeepIPAddressAlways
	c := newCompiler(t, manifests, vms)

	objs, err := c.CompileVirtualMachineSnapshot(context.Background(), testNamespace, "vms")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	var vmip *unstructured.Unstructured
	for i := range objs {
		if objs[i].GetKind() == v1alpha2.VirtualMachineIPAddressKind {
			vmip = &objs[i]
		}
	}
	if vmip == nil {
		t.Fatal("compiled output has no VirtualMachineIPAddress")
	}
	gotType, _, _ := unstructured.NestedString(vmip.Object, "spec", "type")
	gotStatic, _, _ := unstructured.NestedString(vmip.Object, "spec", "staticIP")
	if gotType != string(v1alpha2.VirtualMachineIPAddressTypeStatic) || gotStatic != "10.66.10.14" {
		t.Errorf("spec = {type: %q, staticIP: %q}, want {Static, 10.66.10.14}", gotType, gotStatic)
	}
	if _, found, _ := unstructured.NestedMap(vmip.Object, "status"); found {
		t.Error("status survived into the restore output")
	}
}

func TestCompileVirtualMachineSnapshot_LinksTheVirtualMachineToItsIPAddress(t *testing.T) {
	manifests := &fakeManifests{byContent: map[string][]unstructured.Unstructured{
		vmsContent: {
			obj(v1alpha2.VirtualMachineKind, "vm"),
			vmipObj("vmip-auto-created", string(v1alpha2.VirtualMachineIPAddressTypeAuto), "10.66.10.14"),
		},
	}}
	vms := vmSnapshot()
	vms.Spec.KeepIPAddress = v1alpha2.KeepIPAddressAlways
	c := newCompiler(t, manifests, vms)

	objs, err := c.CompileVirtualMachineSnapshot(context.Background(), testNamespace, "vms")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	for i := range objs {
		if objs[i].GetKind() != v1alpha2.VirtualMachineKind {
			continue
		}
		got, _, _ := unstructured.NestedString(objs[i].Object, "spec", "virtualMachineIPAddressName")
		if got != "vmip-auto-created" {
			t.Fatalf("spec.virtualMachineIPAddressName = %q, want %q", got, "vmip-auto-created")
		}
		return
	}
	t.Fatal("compiled output has no VirtualMachine")
}

// A disk node carries no VirtualMachineIPAddress, so nothing is linked and nothing errors.
func TestLinkCapturedIPAddress_NoIPAddress(t *testing.T) {
	objs := []unstructured.Unstructured{obj(v1alpha2.VirtualMachineKind, "vm")}
	if err := linkCapturedIPAddress(objs); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, found, _ := unstructured.NestedString(objs[0].Object, "spec", "virtualMachineIPAddressName"); found {
		t.Error("spec.virtualMachineIPAddressName was set with no VirtualMachineIPAddress captured")
	}
}

func mainIPAddressObj(specType string) unstructured.Unstructured {
	o := obj(sdnapi.IPAddressKind, "vm-main")
	o.SetAPIVersion(sdnapi.SchemeGroupVersion.String())
	o.SetLabels(map[string]string{annotations.LabelVirtualMachineUID: "old-vm-uid"})
	o.Object["spec"] = map[string]interface{}{
		"type":       specType,
		"networkRef": map[string]interface{}{"kind": "ClusterNetwork", "name": network.MainClusterNetworkName},
	}
	o.Object["status"] = map[string]interface{}{"address": "10.66.10.14"}
	return o
}

func compileVMSnapshotWithMainIPAddress(t *testing.T, keep v1alpha2.KeepIPAddress, capturedIPA unstructured.Unstructured) (vm, ipa *unstructured.Unstructured) {
	t.Helper()
	manifests := &fakeManifests{byContent: map[string][]unstructured.Unstructured{
		vmsContent: {obj(v1alpha2.VirtualMachineKind, "vm"), capturedIPA},
	}}
	vms := vmSnapshot()
	vms.Spec.KeepIPAddress = keep
	objs, err := newCompiler(t, manifests, vms).CompileVirtualMachineSnapshot(context.Background(), testNamespace, "vms")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for i := range objs {
		switch objs[i].GetKind() {
		case v1alpha2.VirtualMachineKind:
			vm = &objs[i]
		case sdnapi.IPAddressKind:
			ipa = &objs[i]
		}
	}
	if vm == nil || ipa == nil {
		t.Fatal("compiled output lacks the VirtualMachine or the IPAddress")
	}
	return vm, ipa
}

func TestCompileVirtualMachineSnapshot_PinsTheCapturedMainIPAddress(t *testing.T) {
	vm, ipa := compileVMSnapshotWithMainIPAddress(t, v1alpha2.KeepIPAddressAlways, mainIPAddressObj("Auto"))

	gotType, _, _ := unstructured.NestedString(ipa.Object, "spec", "type")
	gotIP, _, _ := unstructured.NestedString(ipa.Object, "spec", "static", "ip")
	ignoreTTL, _, _ := unstructured.NestedBool(ipa.Object, "spec", "static", "ignoreLeaseTTLOnAllocation")
	if gotType != sdnapi.IPAddressTypeStatic || gotIP != "10.66.10.14" || !ignoreTTL {
		t.Errorf("spec = {type: %q, static.ip: %q, ignoreLeaseTTLOnAllocation: %v}, want {Static, 10.66.10.14, true}", gotType, gotIP, ignoreTTL)
	}
	if _, found := ipa.GetLabels()[annotations.LabelVirtualMachineUID]; found {
		t.Error("the label of the snapshotted machine survived into the restore output")
	}

	networks, _, _ := unstructured.NestedSlice(vm.Object, "spec", "networks")
	if len(networks) != 1 {
		t.Fatalf("spec.networks = %v, want the Main network only", networks)
	}
	mainNetwork, _ := networks[0].(map[string]interface{})
	if mainNetwork["type"] != string(v1alpha2.NetworksTypeMain) || mainNetwork["ipAddressName"] != "vm-main" {
		t.Errorf("spec.networks[0] = %v, want {type: Main, ipAddressName: vm-main}", mainNetwork)
	}
}

// Never is applied at capture: whatever IPAddress got into the snapshot is restored
// with its address.
func TestCompileVirtualMachineSnapshot_PinsTheCapturedMainIPAddressWithNever(t *testing.T) {
	_, ipa := compileVMSnapshotWithMainIPAddress(t, v1alpha2.KeepIPAddressNever, mainIPAddressObj("Auto"))

	gotType, _, _ := unstructured.NestedString(ipa.Object, "spec", "type")
	if gotType != sdnapi.IPAddressTypeStatic {
		t.Errorf("spec.type = %q, want Static", gotType)
	}
}

func TestPinCapturedMainIPAddress_DropsTheMirrorAnnotation(t *testing.T) {
	ipa := mainIPAddressObj("Static")
	ipa.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: "vmip-uid"})
	if err := pinCapturedMainIPAddress(&ipa); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if _, found := ipa.GetAnnotations()[annotations.AnnMirrorOfVMIP]; found {
		t.Error("the mirror annotation survived into the restore output")
	}
}

func TestLinkCapturedMainIPAddress_KeepsOtherNetworks(t *testing.T) {
	vm := obj(v1alpha2.VirtualMachineKind, "vm")
	vm.Object["spec"] = map[string]interface{}{"networks": []interface{}{
		map[string]interface{}{"type": string(v1alpha2.NetworksTypeMain)},
		map[string]interface{}{"type": string(v1alpha2.NetworksTypeNetwork), "name": "user-net"},
	}}
	objs := []unstructured.Unstructured{vm, mainIPAddressObj("Static")}
	if err := linkCapturedMainIPAddress(objs); err != nil {
		t.Fatalf("link: %v", err)
	}

	networks, _, _ := unstructured.NestedSlice(objs[0].Object, "spec", "networks")
	if len(networks) != 2 {
		t.Fatalf("spec.networks = %v, want two networks", networks)
	}
	if mainNetwork, _ := networks[0].(map[string]interface{}); mainNetwork["ipAddressName"] != "vm-main" {
		t.Errorf("spec.networks[0] = %v, want ipAddressName vm-main", mainNetwork)
	}
}

func TestLinkCapturedMainIPAddress_SkipsWhenVMIPCaptured(t *testing.T) {
	objs := []unstructured.Unstructured{
		obj(v1alpha2.VirtualMachineKind, "vm"),
		vmipObj("vmip", string(v1alpha2.VirtualMachineIPAddressTypeStatic), "10.66.10.14"),
		mainIPAddressObj("Static"),
	}
	if err := linkCapturedMainIPAddress(objs); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, found, _ := unstructured.NestedSlice(objs[0].Object, "spec", "networks"); found {
		t.Error("spec.networks was set although a VirtualMachineIPAddress is captured")
	}
}
