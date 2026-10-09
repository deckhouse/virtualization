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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SetCPUModel", func() {
	newClass := func(cpu v1alpha2.CPU, discovered ...string) *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{CPU: cpu},
			Status: v1alpha2.VirtualMachineClassStatus{
				CpuFeatures: v1alpha2.CpuFeatures{Enabled: discovered},
			},
		}
	}

	cpuOf := func(class *v1alpha2.VirtualMachineClass) *virtv1.CPU {
		b := NewEmptyKVVM(namespacedName("test-vm", "test-ns"), KVVMOptions{})
		Expect(b.SetCPUModel(class)).To(Succeed())
		return b.Resource.Spec.Template.Spec.Domain.CPU
	}

	DescribeTable("node-derived CPU types",
		func(cpuType v1alpha2.CPUType, enableNested *bool, expectedModel string, expectedFeatures []virtv1.CPUFeature) {
			cpu := cpuOf(newClass(v1alpha2.CPU{
				Type:                       cpuType,
				Model:                      "IvyBridge",
				EnableNestedVirtualization: enableNested,
			}))

			Expect(cpu.Model).To(Equal(expectedModel))
			Expect(cpu.Features).To(Equal(expectedFeatures))
		},
		// An unset enableNestedVirtualization must stay indistinguishable from the pre-parameter
		// behaviour: no features at all, so the virtual machines already running keep their spec.
		Entry("Host, nested unset", v1alpha2.CPUTypeHost, nil, virtv1.CPUModeHostModel, nil),
		Entry("Host, nested enabled", v1alpha2.CPUTypeHost, ptr.To(true), virtv1.CPUModeHostModel, nil),
		Entry("Host, nested disabled", v1alpha2.CPUTypeHost, ptr.To(false), virtv1.CPUModeHostModel, []virtv1.CPUFeature{
			{Name: "vmx", Policy: "disable"},
			{Name: "svm", Policy: "disable"},
		}),
		Entry("HostPassthrough, nested unset", v1alpha2.CPUTypeHostPassthrough, nil, virtv1.CPUModeHostPassthrough, nil),
		Entry("HostPassthrough, nested disabled", v1alpha2.CPUTypeHostPassthrough, ptr.To(false), virtv1.CPUModeHostPassthrough, []virtv1.CPUFeature{
			{Name: "vmx", Policy: "disable"},
			{Name: "svm", Policy: "disable"},
		}),
		Entry("Model, nested unset", v1alpha2.CPUTypeModel, nil, "IvyBridge", nil),
		Entry("Model, nested disabled", v1alpha2.CPUTypeModel, ptr.To(false), "IvyBridge", []virtv1.CPUFeature{
			{Name: "vmx", Policy: "disable"},
			{Name: "svm", Policy: "disable"},
		}),
	)

	DescribeTable("generic model CPU types",
		func(enableNested *bool, discovered []string, expectedFeatures []virtv1.CPUFeature) {
			cpu := cpuOf(newClass(v1alpha2.CPU{
				Type:                       v1alpha2.CPUTypeDiscovery,
				EnableNestedVirtualization: enableNested,
			}, discovered...))

			Expect(cpu.Model).To(Equal(GenericCPUModel))
			Expect(cpu.Features).To(Equal(expectedFeatures))
		},
		// With nested virtualization on the feature set is exactly the one of the previous releases:
		// the virtual machines of the existing classes must not see a changed domain spec, which
		// KubeVirt reports as a non-live-updatable change and asks to restart them for.
		Entry("nested unset, nodes expose neither vmx nor svm",
			nil, []string{"mmx", "sse2"},
			[]virtv1.CPUFeature{
				{Name: "mmx", Policy: "require"},
				{Name: "sse2", Policy: "require"},
				{Name: "svm", Policy: "optional"},
			},
		),
		Entry("nested unset, nodes expose vmx",
			nil, []string{"mmx", "vmx"},
			[]virtv1.CPUFeature{
				{Name: "mmx", Policy: "require"},
				{Name: "vmx", Policy: "require"},
				{Name: "svm", Policy: "optional"},
			},
		),
		Entry("nested unset, nodes expose svm",
			nil, []string{"mmx", "svm"},
			[]virtv1.CPUFeature{
				{Name: "mmx", Policy: "require"},
				{Name: "svm", Policy: "require"},
			},
		),
		// A status persisted before discovery started leaving vmx out of the model still names it;
		// the feature is disabled either way.
		Entry("nested disabled, status still lists vmx",
			ptr.To(false), []string{"mmx", "vmx"},
			[]virtv1.CPUFeature{
				{Name: "mmx", Policy: "require"},
				{Name: "vmx", Policy: "disable"},
				{Name: "svm", Policy: "disable"},
			},
		),
		// The regular shape with nested virtualization off: discovery leaves vmx and svm out of
		// status.cpuFeatures.enabled. Both features are still named, as an omitted feature is only
		// off by the current libvirt defaults, which we do not want to depend on.
		Entry("nested disabled, model lists neither vmx nor svm",
			ptr.To(false), []string{"mmx"},
			[]virtv1.CPUFeature{
				{Name: "mmx", Policy: "require"},
				{Name: "vmx", Policy: "disable"},
				{Name: "svm", Policy: "disable"},
			},
		),
		// invtsc pins the virtual machine to the TSC frequency of its node, so it stays optional
		// regardless of nested virtualization.
		Entry("invtsc is never required",
			ptr.To(false), []string{"invtsc"},
			[]virtv1.CPUFeature{
				{Name: "invtsc", Policy: "optional"},
				{Name: "vmx", Policy: "disable"},
				{Name: "svm", Policy: "disable"},
			},
		),
	)

	It("drops the features left by the previous vmclass", func() {
		b := NewEmptyKVVM(namespacedName("test-vm", "test-ns"), KVVMOptions{})
		Expect(b.SetCPUModel(newClass(v1alpha2.CPU{Type: v1alpha2.CPUTypeDiscovery}, "mmx"))).To(Succeed())
		Expect(b.SetCPUModel(newClass(v1alpha2.CPU{Type: v1alpha2.CPUTypeHost}))).To(Succeed())

		Expect(b.Resource.Spec.Template.Spec.Domain.CPU.Features).To(BeEmpty())
	})

	It("rejects an unknown CPU type", func() {
		b := NewEmptyKVVM(namespacedName("test-vm", "test-ns"), KVVMOptions{})
		Expect(b.SetCPUModel(newClass(v1alpha2.CPU{Type: "Unknown"}))).NotTo(Succeed())
	})
})

var _ = Describe("cpuFeaturePolicy", func() {
	DescribeTable("policy of a feature the class provides",
		func(name string, nestedVirtualizationEnabled bool, expected string) {
			Expect(cpuFeaturePolicy(name, nestedVirtualizationEnabled)).To(Equal(expected))
		},
		Entry("svm, nested enabled", "svm", true, cpuFeaturePolicyRequire),
		Entry("svm, nested disabled", "svm", false, cpuFeaturePolicyDisable),
		Entry("vmx, nested enabled", "vmx", true, cpuFeaturePolicyRequire),
		Entry("vmx, nested disabled", "vmx", false, cpuFeaturePolicyDisable),
		Entry("invtsc is never required", "invtsc", true, cpuFeaturePolicyOptional),
		Entry("any other feature is required", "mmx", true, cpuFeaturePolicyRequire),
		Entry("any other feature is required with nested disabled", "mmx", false, cpuFeaturePolicyRequire),
	)
})
