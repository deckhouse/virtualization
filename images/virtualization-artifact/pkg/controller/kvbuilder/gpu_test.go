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
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("GPU", func() {
	It("should render DRA GPU resource claims", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})

		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{Name: "gpu0", Model: "NVIDIA H100"}})
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.ResourceClaims[0].Name).To(Equal("gpu-gpu0"))
		Expect(*res.Spec.Template.Spec.ResourceClaims[0].ResourceClaimTemplateName).To(Equal("vm-a-gpu0"))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[0].Name).To(Equal("gpu-gpu0"))
		Expect(*res.Spec.Template.Spec.Domain.Devices.GPUs[0].ClaimName).To(Equal("gpu-gpu0"))
		Expect(*res.Spec.Template.Spec.Domain.Devices.GPUs[0].RequestName).To(Equal("gpu-gpu0"))
		Expect(res.Annotations).To(BeEmpty())
	})

	It("should render DRA GPU resource claims in stable order", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})

		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{
			{Name: "gpu1", Model: "NVIDIA H100"},
			{Name: "gpu0", Model: "NVIDIA A100-SXM4-40GB"},
		})
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(HaveLen(2))
		Expect(res.Spec.Template.Spec.ResourceClaims[0].Name).To(Equal("gpu-gpu0"))
		Expect(res.Spec.Template.Spec.ResourceClaims[1].Name).To(Equal("gpu-gpu1"))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(HaveLen(2))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[0].Name).To(Equal("gpu-gpu0"))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[1].Name).To(Equal("gpu-gpu1"))
	})

	It("should replace rendered DRA GPU resource claims", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})
		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{Name: "gpu0", Model: "NVIDIA H100"}})

		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{Name: "gpu1", Model: "NVIDIA A100-SXM4-40GB"}})
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.ResourceClaims[0].Name).To(Equal("gpu-gpu1"))
		Expect(*res.Spec.Template.Spec.ResourceClaims[0].ResourceClaimTemplateName).To(Equal("vm-a-gpu1"))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[0].Name).To(Equal("gpu-gpu1"))
	})

	It("should remove rendered DRA GPU resource claims", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})
		kvvm.SetGPUDevices("vm-a", []v1alpha2.GPUDeviceSpec{{Name: "gpu0", Model: "NVIDIA H100"}})

		kvvm.SetGPUDevices("vm-a", nil)
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(BeEmpty())
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(BeEmpty())
		Expect(res.Annotations).To(BeEmpty())
	})
})
