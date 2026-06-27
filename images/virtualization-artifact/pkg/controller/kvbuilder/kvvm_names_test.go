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
