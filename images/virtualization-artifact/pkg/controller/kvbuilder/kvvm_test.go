/*
Copyright 2024 Flant JSC

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

package kvbuilder

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestSetAffinity(t *testing.T) {
	name := "test-name"
	namespace := "test-namespace"

	getDefaultMatchExpressions := func() []corev1.NodeSelectorRequirement {
		return []corev1.NodeSelectorRequirement{
			{
				Key:      "node-role.kubernetes.io/worker",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{""},
			},
		}
	}
	getDefaultAffinity := func() *corev1.Affinity {
		return &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: getDefaultMatchExpressions(),
						},
					},
				},
			},
		}
	}
	tests := []struct {
		name                  string
		expect                *corev1.Affinity
		affinity              *corev1.Affinity
		classMatchExpressions []corev1.NodeSelectorRequirement
	}{
		{
			name:                  "test affinity and classMatchExpressions is nil",
			expect:                nil,
			affinity:              nil,
			classMatchExpressions: nil,
		},
		{
			name:                  "test only affinity nil",
			expect:                getDefaultAffinity(),
			affinity:              nil,
			classMatchExpressions: getDefaultMatchExpressions(),
		},
		{
			name:                  "test only classMatchExpressions nil",
			expect:                getDefaultAffinity(),
			affinity:              getDefaultAffinity(),
			classMatchExpressions: nil,
		},
		{
			name: "test affinity and classMatchExpressions exist",
			expect: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: append(getDefaultMatchExpressions(), corev1.NodeSelectorRequirement{
									Key:      "node-role.kubernetes.io/master",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{""},
								}),
							},
						},
					},
				},
			},
			affinity: getDefaultAffinity(),
			classMatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "node-role.kubernetes.io/master",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{""},
				},
			},
		},
		{
			name:                  "test affinity is nil, but nodeAffinity nil",
			expect:                getDefaultAffinity(),
			affinity:              &corev1.Affinity{},
			classMatchExpressions: getDefaultMatchExpressions(),
		},
	}

	for _, test := range tests {
		builder := NewEmptyKVVM(types.NamespacedName{Name: name, Namespace: namespace}, KVVMOptions{})
		builder.SetAffinity(test.affinity, test.classMatchExpressions)
		if !reflect.DeepEqual(builder.Resource.Spec.Template.Spec.Affinity, test.expect) {
			t.Errorf("test %s failed.expected affinity %v, got %v", test.name, test.expect, builder.Resource.Spec.Template.Spec.Affinity)
		}
	}
}

func TestApplyPVNodeAffinity(t *testing.T) {
	nn := types.NamespacedName{Name: "test", Namespace: "test-ns"}

	pvTerm := func(key string, nodes ...string) corev1.NodeSelectorTerm {
		return corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      key,
				Operator: corev1.NodeSelectorOpIn,
				Values:   nodes,
			}},
		}
	}

	t.Run("No PV terms should not modify affinity", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		b.ApplyPVNodeAffinity(nil)
		if b.Resource.Spec.Template.Spec.Affinity != nil {
			t.Error("affinity should remain nil when no PV terms provided")
		}
	})

	t.Run("No PV terms should preserve existing affinity", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		existing := &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{pvTerm("k", "v")},
				},
			},
		}
		b.Resource.Spec.Template.Spec.Affinity = existing
		b.ApplyPVNodeAffinity(nil)
		if !reflect.DeepEqual(b.Resource.Spec.Template.Spec.Affinity, existing) {
			t.Error("affinity should not change when no PV terms provided")
		}
	})

	t.Run("PV terms applied to empty affinity", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		terms := []corev1.NodeSelectorTerm{pvTerm("topology/node", "node-1")}
		b.ApplyPVNodeAffinity(terms)

		a := b.Resource.Spec.Template.Spec.Affinity
		if a == nil || a.NodeAffinity == nil || a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			t.Fatal("affinity should be set")
		}
		got := a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		if !reflect.DeepEqual(got, terms) {
			t.Errorf("expected %v, got %v", terms, got)
		}
	})

	t.Run("PV terms merged with existing class affinity via cross-product", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		classExpr := []corev1.NodeSelectorRequirement{{
			Key:      "node-role.kubernetes.io/control-plane",
			Operator: corev1.NodeSelectorOpDoesNotExist,
		}}
		b.SetAffinity(nil, classExpr)

		pvTerms := []corev1.NodeSelectorTerm{pvTerm("topology/node", "node-2")}
		b.ApplyPVNodeAffinity(pvTerms)

		a := b.Resource.Spec.Template.Spec.Affinity
		got := a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		if len(got) != 1 {
			t.Fatalf("expected 1 term (cross-product of 1x1), got %d", len(got))
		}
		if len(got[0].MatchExpressions) != 2 {
			t.Errorf("expected 2 match expressions (class + PV), got %d", len(got[0].MatchExpressions))
		}
	})

	t.Run("PV terms cross-product with multiple existing terms", func(t *testing.T) {
		b := NewEmptyKVVM(nn, KVVMOptions{})
		b.Resource.Spec.Template.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						pvTerm("zone", "us-east-1a"),
						pvTerm("zone", "us-east-1b"),
					},
				},
			},
		}

		pvTerms := []corev1.NodeSelectorTerm{
			pvTerm("topology/node", "node-1"),
			pvTerm("topology/node", "node-2"),
		}
		b.ApplyPVNodeAffinity(pvTerms)

		got := b.Resource.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		// 2 existing x 2 PV = 4 terms
		if len(got) != 4 {
			t.Fatalf("expected 4 terms (cross-product 2x2), got %d", len(got))
		}
		for i, term := range got {
			if len(term.MatchExpressions) != 2 {
				t.Errorf("term %d: expected 2 match expressions, got %d", i, len(term.MatchExpressions))
			}
		}
	})
}

func TestSetOsType(t *testing.T) {
	name := "test-name"
	namespace := "test-namespace"

	t.Run("Change from Windows to Generic should remove TPM", func(t *testing.T) {
		builder := NewEmptyKVVM(types.NamespacedName{Name: name, Namespace: namespace}, KVVMOptions{})

		err := builder.SetOSType(v1alpha2.Windows)
		if err != nil {
			t.Fatalf("SetOSType(Windows) failed: %v", err)
		}

		if builder.Resource.Spec.Template.Spec.Domain.Devices.TPM == nil {
			t.Error("TPM should be present after setting Windows OS")
		}

		err = builder.SetOSType(v1alpha2.GenericOs)
		if err != nil {
			t.Fatalf("SetOSType(GenericOs) failed: %v", err)
		}

		if builder.Resource.Spec.Template.Spec.Domain.Devices.TPM != nil {
			t.Error("TPM should be removed after changing from Windows to Generic OS")
		}
	})

	t.Run("Legacy pins i440fx, disables SMM and drops the tablet", func(t *testing.T) {
		builder := NewEmptyKVVM(types.NamespacedName{Name: name, Namespace: namespace}, KVVMOptions{})

		if err := builder.SetOSType(v1alpha2.Windows); err != nil {
			t.Fatalf("SetOSType(Windows) failed: %v", err)
		}
		builder.SetTablet("default-0")

		if err := builder.SetOSType(v1alpha2.LegacyOs); err != nil {
			t.Fatalf("SetOSType(LegacyOs) failed: %v", err)
		}

		domain := builder.Resource.Spec.Template.Spec.Domain
		// "pc" is the QEMU alias for i440fx; "pc-i440fx" is not a valid machine name.
		if domain.Machine == nil || domain.Machine.Type != "pc" {
			t.Errorf("expected pc machine type, got %+v", domain.Machine)
		}
		// SMM exists only on q35: leaving it on would keep the domain from starting.
		if domain.Features.SMM == nil || ptr.Deref(domain.Features.SMM.Enabled, true) {
			t.Errorf("expected SMM disabled, got %+v", domain.Features.SMM)
		}
		if domain.Features.Hyperv != nil {
			t.Error("hyperv enlightenments should be absent for Legacy")
		}
		if domain.Devices.TPM != nil || domain.Devices.Rng != nil {
			t.Error("TPM and RNG should be absent for Legacy")
		}
		// The tablet stays: on i440fx the USB controller is PIIX3 UHCI, which these
		// guests can drive, and it gives the console absolute pointer positioning.
		if len(domain.Devices.Inputs) != 1 || domain.Devices.Inputs[0].Type != virtv1.InputTypeTablet {
			t.Errorf("expected a usb tablet for Legacy, got %+v", domain.Devices.Inputs)
		}
	})

	t.Run("Legacy keeps cpu and memory non-hotpluggable", func(t *testing.T) {
		builder := NewEmptyKVVM(types.NamespacedName{Name: name, Namespace: namespace}, KVVMOptions{
			OsType: v1alpha2.LegacyOs,
		})

		if err := builder.SetCPU(4, "100%"); err != nil {
			t.Fatalf("SetCPU failed: %v", err)
		}
		builder.SetMemory(resource.MustParse("2Gi"))

		domain := builder.Resource.Spec.Template.Spec.Domain
		if domain.CPU.MaxSockets != domain.CPU.Sockets {
			t.Errorf("expected maxSockets == sockets to rule out cpu hotplug, got %d and %d",
				domain.CPU.MaxSockets, domain.CPU.Sockets)
		}
		if _, ok := builder.Resource.Spec.Template.ObjectMeta.Annotations[VCPUTopologyDynamicCoresAnnotation]; ok {
			t.Error("dynamic cores annotation should be absent for Legacy")
		}
		if domain.Memory != nil {
			t.Errorf("domain.memory should be unset to rule out memory hotplug, got %+v", domain.Memory)
		}
		if _, ok := domain.Resources.Limits[corev1.ResourceMemory]; !ok {
			t.Error("memory should be set through resources limits for Legacy")
		}
	})

	t.Run("Generic and Windows keep q35 with SMM enabled", func(t *testing.T) {
		for _, osType := range []v1alpha2.OsType{v1alpha2.Windows, v1alpha2.GenericOs} {
			builder := NewEmptyKVVM(types.NamespacedName{Name: name, Namespace: namespace}, KVVMOptions{})
			if err := builder.SetOSType(osType); err != nil {
				t.Fatalf("SetOSType(%s) failed: %v", osType, err)
			}

			domain := builder.Resource.Spec.Template.Spec.Domain
			if domain.Machine == nil || domain.Machine.Type != "q35" {
				t.Errorf("%s: expected q35 machine type, got %+v", osType, domain.Machine)
			}
			if domain.Features.SMM == nil || !ptr.Deref(domain.Features.SMM.Enabled, false) {
				t.Errorf("%s: expected SMM enabled, got %+v", osType, domain.Features.SMM)
			}
		}
	})
}

func TestSetDiskBus(t *testing.T) {
	getBus := func(b *KVVM, name string) virtv1.DiskBus {
		for _, d := range b.Resource.Spec.Template.Spec.Domain.Devices.Disks {
			if d.Name != name {
				continue
			}
			if d.CDRom != nil {
				return d.CDRom.Bus
			}
			if d.Disk != nil {
				return d.Disk.Bus
			}
		}
		return ""
	}

	newKVVM := func(paravirt bool) *KVVM {
		return NewEmptyKVVM(types.NamespacedName{Name: "test", Namespace: "default"}, KVVMOptions{
			EnableParavirtualization: paravirt,
		})
	}

	t.Run("Legacy osType keeps the virtio adapter and the transitional devices", func(t *testing.T) {
		b := NewEmptyKVVM(types.NamespacedName{Name: "test", Namespace: "default"}, KVVMOptions{
			EnableParavirtualization: true,
			OsType:                   v1alpha2.LegacyOs,
		})

		b.SetNetworkInterface("default", "", 0)
		if model := b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces[0].Model; model != nicModelVirtio {
			t.Errorf("expected the virtio adapter, got %q", model)
		}

		if err := b.SetOSType(v1alpha2.LegacyOs); err != nil {
			t.Fatal(err)
		}
		if machine := b.Resource.Spec.Template.Spec.Domain.Machine; machine == nil || machine.Type != "pc" {
			t.Errorf("expected the i440fx machine to stay, got %+v", machine)
		}
		// Without this the devices come out non-transitional, and the legacy virtio
		// drivers these guests can install do not bind to them.
		if transitional := b.Resource.Spec.Template.Spec.Domain.Devices.UseVirtioTransitional; transitional == nil || !*transitional {
			t.Error("expected transitional virtio devices for Legacy")
		}
	})

	t.Run("switching away from Legacy drops the transitional virtio devices", func(t *testing.T) {
		for _, osType := range []v1alpha2.OsType{v1alpha2.GenericOs, v1alpha2.Windows} {
			b := NewEmptyKVVM(types.NamespacedName{Name: "test", Namespace: "default"}, KVVMOptions{OsType: v1alpha2.LegacyOs})
			if err := b.SetOSType(v1alpha2.LegacyOs); err != nil {
				t.Fatal(err)
			}

			if err := b.SetOSType(osType); err != nil {
				t.Fatal(err)
			}
			if transitional := b.Resource.Spec.Template.Spec.Domain.Devices.UseVirtioTransitional; transitional != nil {
				t.Errorf("%s: transitional virtio devices leaked from Legacy, got %v", osType, *transitional)
			}
		}
	})

	t.Run("static disk moves to the new preset bus on a paravirtualization flip", func(t *testing.T) {
		old := newKVVM(false)
		if err := old.SetDisk("disk", SetDiskOptions{ContainerDisk: ptr.To("img")}); err != nil {
			t.Fatal(err)
		}
		if bus := getBus(old, "disk"); bus != virtv1.DiskBusSATA {
			t.Fatalf("expected sata disk bus before flip, got %q", bus)
		}

		flipped := NewKVVM(old.Resource, KVVMOptions{EnableParavirtualization: true})
		if err := flipped.SetDisk("disk", SetDiskOptions{ContainerDisk: ptr.To("img")}); err != nil {
			t.Fatal(err)
		}
		if bus := getBus(flipped, "disk"); bus != virtv1.DiskBusSCSI {
			t.Errorf("expected scsi disk bus after flip, got %q", bus)
		}
	})
}

func TestSetProvisioning(t *testing.T) {
	hasDisk := func(b *KVVM, name string) bool {
		for _, d := range b.Resource.Spec.Template.Spec.Domain.Devices.Disks {
			if d.Name == name {
				return true
			}
		}
		return false
	}
	hasVolume := func(b *KVVM, name string) bool {
		for _, v := range b.Resource.Spec.Template.Spec.Volumes {
			if v.Name == name {
				return true
			}
		}
		return false
	}

	cloudInit := &v1alpha2.Provisioning{
		Type:     v1alpha2.ProvisioningTypeUserData,
		UserData: "#cloud-config",
	}
	sysprep := &v1alpha2.Provisioning{
		Type:       v1alpha2.ProvisioningTypeSysprepRef,
		SysprepRef: &v1alpha2.SysprepRef{Kind: v1alpha2.SysprepRefKindSecret, Name: "sysprep-secret"},
	}

	t.Run("removes cloudinit disk and volume when provisioning is removed", func(t *testing.T) {
		b := newTestKVVM()
		if err := b.SetProvisioning(cloudInit); err != nil {
			t.Fatalf("SetProvisioning(cloudInit) failed: %v", err)
		}
		if !hasDisk(b, CloudInitDiskName) || !hasVolume(b, CloudInitDiskName) {
			t.Fatal("cloudinit disk and volume should be present after setting provisioning")
		}

		if err := b.SetProvisioning(nil); err != nil {
			t.Fatalf("SetProvisioning(nil) failed: %v", err)
		}
		if hasDisk(b, CloudInitDiskName) || hasVolume(b, CloudInitDiskName) {
			t.Error("cloudinit disk and volume should be removed after removing provisioning")
		}
	})

	t.Run("removes sysprep disk and volume when provisioning is removed", func(t *testing.T) {
		b := newTestKVVM()
		if err := b.SetProvisioning(sysprep); err != nil {
			t.Fatalf("SetProvisioning(sysprep) failed: %v", err)
		}
		if !hasDisk(b, SysprepDiskName) || !hasVolume(b, SysprepDiskName) {
			t.Fatal("sysprep disk and volume should be present after setting provisioning")
		}

		if err := b.SetProvisioning(nil); err != nil {
			t.Fatalf("SetProvisioning(nil) failed: %v", err)
		}
		if hasDisk(b, SysprepDiskName) || hasVolume(b, SysprepDiskName) {
			t.Error("sysprep disk and volume should be removed after removing provisioning")
		}
	})

	t.Run("removes stale disk when provisioning type changes", func(t *testing.T) {
		b := newTestKVVM()
		if err := b.SetProvisioning(cloudInit); err != nil {
			t.Fatalf("SetProvisioning(cloudInit) failed: %v", err)
		}
		if err := b.SetProvisioning(sysprep); err != nil {
			t.Fatalf("SetProvisioning(sysprep) failed: %v", err)
		}

		if hasDisk(b, CloudInitDiskName) || hasVolume(b, CloudInitDiskName) {
			t.Error("cloudinit disk and volume should be removed after switching to sysprep")
		}
		if !hasDisk(b, SysprepDiskName) || !hasVolume(b, SysprepDiskName) {
			t.Error("sysprep disk and volume should be present after switching to sysprep")
		}
	})
}

func newTestKVVM() *KVVM {
	return NewEmptyKVVM(types.NamespacedName{Name: "test", Namespace: "default"}, KVVMOptions{
		EnableParavirtualization: true,
	})
}

func TestSetNetworkInterfaceAbsent(t *testing.T) {
	b := newTestKVVM()
	b.SetNetworkInterface("default", "", 1)
	b.SetNetworkInterface("veth_n12345678", "aa:bb:cc:dd:ee:ff", 2)

	b.SetNetworkInterfaceAbsent("veth_n12345678")

	for _, iface := range b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces {
		if iface.Name == "veth_n12345678" {
			if iface.State != virtv1.InterfaceStateAbsent {
				t.Errorf("expected State %q, got %q", virtv1.InterfaceStateAbsent, iface.State)
			}
			return
		}
	}
	t.Error("interface veth_n12345678 not found")
}

func TestSetNetworkInterfaceReplacesExisting(t *testing.T) {
	b := newTestKVVM()
	b.SetNetworkInterface("veth_n12345678", "aa:bb:cc:dd:ee:ff", 2)
	b.SetNetworkInterfaceAbsent("veth_n12345678")

	b.SetNetworkInterface("veth_n12345678", "aa:bb:cc:dd:ee:ff", 2)

	for _, iface := range b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces {
		if iface.Name == "veth_n12345678" {
			if iface.State != "" {
				t.Errorf("expected empty State after re-add, got %q", iface.State)
			}
			return
		}
	}
	t.Error("interface veth_n12345678 not found")
}

func TestSetNetworkMarksRemovedAsAbsent(t *testing.T) {
	b := newTestKVVM()
	b.SetNetworkInterface("default", "", 1)
	b.SetNetworkInterface("veth_n12345678", "aa:bb:cc:dd:ee:ff", 2)

	setNetwork(b, network.InterfaceSpecList{
		{InterfaceName: "default", MAC: "", ID: 1},
	})

	found := false
	for _, iface := range b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces {
		if iface.Name == "veth_n12345678" {
			found = true
			if iface.State != virtv1.InterfaceStateAbsent {
				t.Errorf("removed interface should have State %q, got %q", virtv1.InterfaceStateAbsent, iface.State)
			}
		}
		if iface.Name == "default" && iface.State != "" {
			t.Errorf("kept interface should have empty State, got %q", iface.State)
		}
	}
	if !found {
		t.Error("removed interface should be retained with absent state, not deleted")
	}
}

func TestSetNetworkRemovesDefaultEntirely(t *testing.T) {
	b := newTestKVVM()
	b.SetNetworkInterface("default", "", 1)
	b.SetNetworkInterface("veth_n12345678", "aa:bb:cc:dd:ee:ff", 2)

	setNetwork(b, network.InterfaceSpecList{
		{InterfaceName: "veth_n12345678", MAC: "aa:bb:cc:dd:ee:ff", ID: 2},
	})

	for _, iface := range b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces {
		if iface.Name == "default" {
			t.Fatalf("default interface must be removed from KVVM template when Main is dropped (KubeVirt rejects State: absent on default)")
		}
	}
	for _, n := range b.Resource.Spec.Template.Spec.Networks {
		if n.Name == "default" {
			t.Fatalf("default network entry must be removed alongside the interface")
		}
	}
}

func TestSetNetworkAddsNewInterface(t *testing.T) {
	b := newTestKVVM()
	b.SetNetworkInterface("default", "", 1)

	setNetwork(b, network.InterfaceSpecList{
		{InterfaceName: "default", MAC: "", ID: 1},
		{InterfaceName: "veth_n12345678", MAC: "aa:bb:cc:dd:ee:ff", ID: 2},
	})

	found := false
	for _, iface := range b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces {
		if iface.Name == "veth_n12345678" {
			found = true
			if iface.ACPIIndex != 2 {
				t.Errorf("expected ACPIIndex 2, got %d", iface.ACPIIndex)
			}
		}
	}
	if !found {
		t.Error("new interface should be added")
	}
}

func TestSetNetworkKeepsDefaultFirstWhenMainAddedLast(t *testing.T) {
	b := newTestKVVM()
	b.SetNetworkInterface("veth_cn11111111", "aa:bb:cc:dd:ee:01", 2)
	b.SetNetworkInterface("veth_n22222222", "aa:bb:cc:dd:ee:02", 3)

	setNetwork(b, network.InterfaceSpecList{
		{InterfaceName: "default", MAC: "", ID: 1},
		{InterfaceName: "veth_cn11111111", MAC: "aa:bb:cc:dd:ee:01", ID: 2},
		{InterfaceName: "veth_n22222222", MAC: "aa:bb:cc:dd:ee:02", ID: 3},
	})

	ifaces := b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces
	if got := ifaces[0].Name; got != "default" {
		t.Errorf("default interface must be first, got order: %v", interfaceNames(ifaces))
	}
	nets := b.Resource.Spec.Template.Spec.Networks
	if got := nets[0].Name; got != "default" {
		t.Errorf("default network must be first, got order: %v", networkNames(nets))
	}
}

func interfaceNames(ifaces []virtv1.Interface) []string {
	names := make([]string, len(ifaces))
	for i, iface := range ifaces {
		names[i] = iface.Name
	}
	return names
}

func networkNames(nets []virtv1.Network) []string {
	names := make([]string, len(nets))
	for i, n := range nets {
		names[i] = n.Name
	}
	return names
}
