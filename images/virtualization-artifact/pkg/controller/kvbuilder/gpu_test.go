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
)

var _ = Describe("GPU", func() {
	It("should render DRA GPU resource claim", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})

		kvvm.SetGPU("vm-a", "GPU-test")
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.ResourceClaims[0].Name).To(Equal(GPUName))
		Expect(*res.Spec.Template.Spec.ResourceClaims[0].ResourceClaimTemplateName).To(Equal("vm-a-gpu-template"))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(HaveLen(1))
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs[0].Name).To(Equal(GPUName))
		Expect(*res.Spec.Template.Spec.Domain.Devices.GPUs[0].ClaimName).To(Equal(GPUName))
		Expect(*res.Spec.Template.Spec.Domain.Devices.GPUs[0].RequestName).To(Equal(GPUResourceClaimRequestName))
		Expect(res.Annotations).To(HaveKeyWithValue(AppliedGPUAnnotation, "GPU-test"))
	})

	It("should remove rendered DRA GPU resource claim", func() {
		kvvm := NewEmptyKVVM(types.NamespacedName{Name: "vm-a", Namespace: "default"}, KVVMOptions{})
		kvvm.SetGPU("vm-a", "GPU-test")

		kvvm.SetGPU("vm-a", "")
		res := kvvm.GetResource()

		Expect(res.Spec.Template.Spec.ResourceClaims).To(BeEmpty())
		Expect(res.Spec.Template.Spec.Domain.Devices.GPUs).To(BeEmpty())
		Expect(res.Annotations).NotTo(HaveKey(AppliedGPUAnnotation))
	})
})
