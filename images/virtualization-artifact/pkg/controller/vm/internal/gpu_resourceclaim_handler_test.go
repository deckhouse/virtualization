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

package internal

import (
	context "context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("GPUResourceClaimHandler", func() {
	const (
		vmName    = "vm-a"
		namespace = "default"
		gpuID     = "GPU-test"
	)

	newVM := func(id string) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, Annotations: map[string]string{}},
		}
		if id != "" {
			vm.Annotations[annotations.AnnVMGPUID] = id
		}
		return vm
	}

	It("should create GPU ResourceClaimTemplate", func() {
		fakeClient, _, vmState := setupEnvironment(newVM(gpuID))
		handler := NewGPUResourceClaimHandler(fakeClient)

		_, err := handler.Handle(context.Background(), vmState)

		Expect(err).NotTo(HaveOccurred())
		template := &resourcev1.ResourceClaimTemplate{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: kvbuilder.GPUResourceClaimTemplateName(vmName), Namespace: namespace}, template)).To(Succeed())
		Expect(template.Spec.Spec.Devices.Requests).To(HaveLen(1))
		request := template.Spec.Spec.Devices.Requests[0]
		Expect(request.Name).To(Equal(kvbuilder.GPUResourceClaimRequestName))
		Expect(request.Exactly.DeviceClassName).To(Equal(gpuDeviceClassName))
		Expect(request.Exactly.Selectors[0].CEL.Expression).To(ContainSubstring(`gpuUUID == "GPU-test"`))
		Expect(request.Exactly.Selectors[0].CEL.Expression).To(ContainSubstring(`deviceType == "physical"`))
		Expect(request.Exactly.Selectors[0].CEL.Expression).To(ContainSubstring(`!has(device.attributes["gpu.deckhouse.io"].sharingStrategy)`))
	})

	It("should delete owned GPU ResourceClaimTemplate when annotation is removed", func() {
		vm := newVM("")
		template := buildGPUResourceClaimTemplate(vm, kvbuilder.GPUResourceClaimTemplateName(vmName), buildGPUResourceClaimTemplateSpec(gpuID))
		fakeClient, _, vmState := setupEnvironment(vm, template)
		handler := NewGPUResourceClaimHandler(fakeClient)

		_, err := handler.Handle(context.Background(), vmState)

		Expect(err).NotTo(HaveOccurred())
		stored := &resourcev1.ResourceClaimTemplate{}
		err = fakeClient.Get(context.Background(), types.NamespacedName{Name: kvbuilder.GPUResourceClaimTemplateName(vmName), Namespace: namespace}, stored)
		Expect(err).To(HaveOccurred())
	})
})
