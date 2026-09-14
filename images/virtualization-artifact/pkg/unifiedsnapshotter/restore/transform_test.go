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
