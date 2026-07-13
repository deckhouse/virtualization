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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("GPU", func() {
	It("should render DRA GPU resource claims", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})

		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{GPUClassName: "nvidia-h100"}})
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.ResourceClaims[0].Name).To(Equal("gpu-0"))
		Expect(*res.Spec.Template.Spec.ResourceClaims[0].ResourceClaimTemplateName).To(Equal(GPUResourceClaimTemplateName("vm-a", 0)))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[0].Name).To(Equal("gpu-0"))
		Expect(*res.Spec.Template.Spec.Domain.Devices.GPUs[0].ClaimName).To(Equal("gpu-0"))
		Expect(*res.Spec.Template.Spec.Domain.Devices.GPUs[0].RequestName).To(Equal("gpu-0"))
	})

	It("should index claims by GPUClass order regardless of spec order", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})

		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{
			{GPUClassName: "nvidia-h100"},
			{GPUClassName: "nvidia-a100"},
		})
		res := kvvm.GetResource()

		// Sorted by GPUClass: nvidia-a100 -> index 0, nvidia-h100 -> index 1.
		Expect(res.Spec.Template.Spec.ResourceClaims).To(HaveLen(2))
		Expect(res.Spec.Template.Spec.ResourceClaims[0].Name).To(Equal("gpu-0"))
		Expect(res.Spec.Template.Spec.ResourceClaims[1].Name).To(Equal("gpu-1"))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(HaveLen(2))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[0].Name).To(Equal("gpu-0"))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[1].Name).To(Equal("gpu-1"))
	})

	It("should replace rendered DRA GPU resource claims", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})
		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{GPUClassName: "nvidia-h100"}})

		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{GPUClassName: "nvidia-a100"}})
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.ResourceClaims[0].Name).To(Equal("gpu-0"))
		Expect(*res.Spec.Template.Spec.ResourceClaims[0].ResourceClaimTemplateName).To(Equal(GPUResourceClaimTemplateName("vm-a", 0)))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[0].Name).To(Equal("gpu-0"))
	})

	It("should not touch resource claims and GPUs it does not own", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})
		res := kvvm.GetResource()
		res.Spec.Template.Spec.ResourceClaims = []virtv1.ResourceClaim{{
			PodResourceClaim: corev1.PodResourceClaim{
				Name:                      "gpu-foreign",
				ResourceClaimTemplateName: ptr.To("foreign-template"),
			},
		}}
		res.Spec.Template.Spec.Domain.Devices.GPUs = []virtv1.GPU{{
			Name:       "gpu-legacy",
			DeviceName: "nvidia.com/GH100",
		}}

		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{GPUClassName: "nvidia-h100"}})
		res = kvvm.GetResource()

		claimNames := make([]string, 0)
		for _, c := range res.Spec.Template.Spec.ResourceClaims {
			claimNames = append(claimNames, c.Name)
		}
		gpuNames := make([]string, 0)
		for _, g := range res.Spec.Template.Spec.Domain.Devices.GPUs {
			gpuNames = append(gpuNames, g.Name)
		}
		Expect(claimNames).To(ConsistOf("gpu-foreign", "gpu-0"))
		Expect(gpuNames).To(ConsistOf("gpu-legacy", "gpu-0"))
	})

	It("should remove rendered DRA GPU resource claims", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})
		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{GPUClassName: "nvidia-h100"}})

		kvvm.SetGPUDevices("vm-a", nil)
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(BeEmpty())
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(BeEmpty())
	})
})
