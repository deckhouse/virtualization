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

package kvbuilder

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Derived KubeVirt disk/volume names", func() {
	DescribeTable("passthrough: short, valid label names are returned unchanged",
		func(name string) {
			// Every name allowed by the previous validation must map to the
			// byte-identical derived name, so existing volumes are never renamed.
			Expect(GenerateVDDiskName(name)).To(Equal("vd-" + name))
		},
		Entry("typical", "web-data"),
		Entry("exactly at budget", strings.Repeat("a", vdNameBudget)),
	)

	It("hashes names over the budget into a valid label within 63 chars", func() {
		name := strings.Repeat("a", vdNameBudget+1)
		got := GenerateVDDiskName(name)

		Expect(got).To(HavePrefix("vd-"))
		Expect(len(got)).To(BeNumerically("<=", 63))
		Expect(kvalidation.IsDNS1123Label(got)).To(BeEmpty())
		Expect(got).NotTo(Equal("vd-" + name))
		Expect(got).To(MatchRegexp(`-[0-9a-f]{16}$`)) // FNV-1a 64-bit suffix
	})

	It("is deterministic for the same input", func() {
		name := strings.Repeat("b", 120)
		Expect(GenerateVDDiskName(name)).To(Equal(GenerateVDDiskName(name)))
	})

	It("allows dots by sanitizing and hashing instead of passthrough", func() {
		got := GenerateVDDiskName("disk.2024.backup")
		Expect(got).NotTo(ContainSubstring("."))
		Expect(kvalidation.IsDNS1123Label(got)).To(BeEmpty())
		Expect(got).To(HavePrefix("vd-disk-2024-backup-"))
	})

	It("does not collide when sanitization makes readable parts equal", func() {
		// The dotted name is hashed; its dashed twin is a valid label (passthrough).
		Expect(GenerateVDDiskName("disk.2024.backup")).
			NotTo(Equal(GenerateVDDiskName("disk-2024-backup")))
	})

	It("does not collide when long names share a truncated prefix", func() {
		base := strings.Repeat("a", vdNameBudget)
		Expect(GenerateVDDiskName(base + "-one")).
			NotTo(Equal(GenerateVDDiskName(base + "-two")))
	})

	DescribeTable("containerDisk container name stays within 63 for long image names",
		func(gen func(string) string) {
			vol := gen(strings.Repeat("a", 200))
			// KubeVirt wraps a containerDisk volume name as "volume<name>-init".
			container := "volume" + vol + "-init"
			Expect(kvalidation.IsDNS1123Label(vol)).To(BeEmpty())
			Expect(len(container)).To(BeNumerically("<=", 63), container)
		},
		Entry("VirtualImage", GenerateVIDiskName),
		Entry("ClusterVirtualImage", GenerateCVIDiskName),
	)

	DescribeTable("GenerateDiskName routes to the kind-specific generator",
		func(kind v1alpha2.BlockDeviceKind, gen func(string) string) {
			name := strings.Repeat("z", 80)
			Expect(GenerateDiskName(kind, name)).To(Equal(gen(name)))
		},
		Entry("disk", v1alpha2.DiskDevice, GenerateVDDiskName),
		Entry("image", v1alpha2.ImageDevice, GenerateVIDiskName),
		Entry("cluster image", v1alpha2.ClusterImageDevice, GenerateCVIDiskName),
	)

	It("round-trips legacy short names through GetOriginalDiskName", func() {
		name, kind := GetOriginalDiskName(GenerateVDDiskName("data-disk"))
		Expect(name).To(Equal("data-disk"))
		Expect(kind).To(Equal(v1alpha2.DiskDevice))
	})
})

var _ = Describe("VolumeNameResolver", func() {
	It("reverses derived names via candidates, including hashed ones", func() {
		longName := strings.Repeat("a", vdNameBudget+10) // forced into the hash branch
		r := NewVolumeNameResolver()
		r.Add(v1alpha2.DiskDevice, longName)
		r.Add(v1alpha2.ImageDevice, "ubuntu")

		name, kind := r.Resolve(GenerateVDDiskName(longName))
		Expect(name).To(Equal(longName))
		Expect(kind).To(Equal(v1alpha2.DiskDevice))

		name, kind = r.Resolve(GenerateVIDiskName("ubuntu"))
		Expect(name).To(Equal("ubuntu"))
		Expect(kind).To(Equal(v1alpha2.ImageDevice))
	})

	It("falls back to prefix-strip for legacy names not among candidates", func() {
		r := NewVolumeNameResolver()
		name, kind := r.Resolve("vd-legacy-disk")
		Expect(name).To(Equal("legacy-disk"))
		Expect(kind).To(Equal(v1alpha2.DiskDevice))
	})

	It("returns empty kind for non block-device volumes", func() {
		r := NewVolumeNameResolver()
		_, kind := r.Resolve("cloudinit")
		Expect(kind).To(BeEmpty())
	})
})

var _ = Describe("detectDiskNameCollisions", func() {
	It("passes for distinct block devices (incl. long and dotted names)", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{Kind: v1alpha2.DiskDevice, Name: "data"},
					{Kind: v1alpha2.DiskDevice, Name: strings.Repeat("a", 80)},
					{Kind: v1alpha2.ImageDevice, Name: "ubuntu"},
					{Kind: v1alpha2.DiskDevice, Name: "disk.with.dots"},
				},
			},
		}
		vmbda := map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment{
			{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: "extra"}: nil,
		}
		Expect(detectDiskNameCollisions(vm, vmbda)).To(Succeed())
	})

	It("does not flag the same resource present in both spec and VMBDA", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{Kind: v1alpha2.DiskDevice, Name: "shared"},
				},
			},
		}
		vmbda := map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment{
			{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: "shared"}: nil,
		}
		Expect(detectDiskNameCollisions(vm, vmbda)).To(Succeed())
	})
})
