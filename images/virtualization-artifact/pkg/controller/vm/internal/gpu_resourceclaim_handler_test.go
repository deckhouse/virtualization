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
		vmName      = "vm-a"
		namespace   = "default"
		deviceClass = "nvidia-h100"
	)

	newVM := func(devices ...v1alpha2.GPUDeviceSpec) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineSpec{GPUDevices: devices},
		}
	}

	It("should create GPU ResourceClaimTemplate", func() {
		fakeClient, _, vmState := setupEnvironment(newVM(v1alpha2.GPUDeviceSpec{Name: "gpu0", GPUClassName: deviceClass}))
		handler := NewGPUResourceClaimHandler(fakeClient)

		_, err := handler.Handle(context.Background(), vmState)

		Expect(err).NotTo(HaveOccurred())
		template := &resourcev1.ResourceClaimTemplate{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: kvbuilder.GPUResourceClaimTemplateName(vmName, "gpu0"), Namespace: namespace}, template)).To(Succeed())
		Expect(template.Spec.Spec.Devices.Requests).To(HaveLen(1))
		request := template.Spec.Spec.Devices.Requests[0]
		Expect(request.Name).To(Equal(kvbuilder.GPUResourceClaimName("gpu0")))
		Expect(request.Exactly.DeviceClassName).To(Equal(deviceClass))
		Expect(request.Exactly.Selectors).To(BeEmpty())
		Expect(template.Spec.Spec.Devices.Config).To(HaveLen(1))
		config := template.Spec.Spec.Devices.Config[0]
		Expect(config.Requests).To(ConsistOf(kvbuilder.GPUResourceClaimName("gpu0")))
		Expect(config.Opaque.Driver).To(Equal(kvbuilder.GPUDRADriverName))
		Expect(string(config.Opaque.Parameters.Raw)).To(ContainSubstring(`"kind":"VfioDeviceConfig"`))
	})

	It("should delete owned GPU ResourceClaimTemplate when device is removed from spec", func() {
		vm := newVM()
		template := buildGPUResourceClaimTemplate(vm, kvbuilder.GPUResourceClaimTemplateName(vmName, "gpu0"), buildGPUResourceClaimTemplateSpec(v1alpha2.GPUDeviceSpec{Name: "gpu0", GPUClassName: deviceClass}))
		fakeClient, _, vmState := setupEnvironment(vm, template)
		handler := NewGPUResourceClaimHandler(fakeClient)

		_, err := handler.Handle(context.Background(), vmState)

		Expect(err).NotTo(HaveOccurred())
		stored := &resourcev1.ResourceClaimTemplate{}
		err = fakeClient.Get(context.Background(), types.NamespacedName{Name: kvbuilder.GPUResourceClaimTemplateName(vmName, "gpu0"), Namespace: namespace}, stored)
		Expect(err).To(HaveOccurred())
	})

	It("should recreate GPU ResourceClaimTemplate when the desired spec changes", func() {
		vm := newVM(v1alpha2.GPUDeviceSpec{Name: "gpu0", GPUClassName: "nvidia-a100"})
		templateName := kvbuilder.GPUResourceClaimTemplateName(vmName, "gpu0")
		template := buildGPUResourceClaimTemplate(vm, templateName, buildGPUResourceClaimTemplateSpec(v1alpha2.GPUDeviceSpec{Name: "gpu0", GPUClassName: deviceClass}))
		fakeClient, _, vmState := setupEnvironment(vm, template)
		handler := NewGPUResourceClaimHandler(fakeClient)

		_, err := handler.Handle(context.Background(), vmState)

		Expect(err).NotTo(HaveOccurred())
		stored := &resourcev1.ResourceClaimTemplate{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: templateName, Namespace: namespace}, stored)).To(Succeed())
		Expect(stored.Spec.Spec.Devices.Requests[0].Exactly.DeviceClassName).To(Equal("nvidia-a100"))
		Expect(stored.Annotations).To(HaveKeyWithValue(annotations.AnnGPUClaimSpecHash, gpuClaimSpecHash(stored.Spec)))
	})

	It("should not recreate GPU ResourceClaimTemplate without hash annotation when spec matches", func() {
		vm := newVM(v1alpha2.GPUDeviceSpec{Name: "gpu0", GPUClassName: deviceClass})
		templateName := kvbuilder.GPUResourceClaimTemplateName(vmName, "gpu0")
		template := buildGPUResourceClaimTemplate(vm, templateName, buildGPUResourceClaimTemplateSpec(v1alpha2.GPUDeviceSpec{Name: "gpu0", GPUClassName: deviceClass}))
		template.Annotations = nil
		template.Labels = map[string]string{"keep": "me"}
		fakeClient, _, vmState := setupEnvironment(vm, template)
		handler := NewGPUResourceClaimHandler(fakeClient)

		_, err := handler.Handle(context.Background(), vmState)

		Expect(err).NotTo(HaveOccurred())
		stored := &resourcev1.ResourceClaimTemplate{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: templateName, Namespace: namespace}, stored)).To(Succeed())
		Expect(stored.Labels).To(HaveKeyWithValue("keep", "me"))
		Expect(stored.Annotations).NotTo(HaveKey(annotations.AnnGPUClaimSpecHash))
	})

	It("should not replace GPU ResourceClaimTemplate owned by another controller", func() {
		vm := newVM(v1alpha2.GPUDeviceSpec{Name: "gpu0", GPUClassName: deviceClass})
		template := &resourcev1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: kvbuilder.GPUResourceClaimTemplateName(vmName, "gpu0"), Namespace: namespace},
		}
		fakeClient, _, vmState := setupEnvironment(vm, template)
		handler := NewGPUResourceClaimHandler(fakeClient)

		_, err := handler.Handle(context.Background(), vmState)

		Expect(err).To(HaveOccurred())
		stored := &resourcev1.ResourceClaimTemplate{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: template.Name, Namespace: namespace}, stored)).To(Succeed())
		Expect(stored.OwnerReferences).To(BeEmpty())
	})

	It("should not delete a ResourceClaimTemplate owned by another VM with a shared name prefix", func() {
		vm := newVM()
		vm.UID = "uid-vm-a"
		otherVM := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName + "-b", Namespace: namespace, UID: "uid-vm-a-b"},
		}
		otherName := kvbuilder.GPUResourceClaimTemplateName(otherVM.Name, "gpu0")
		Expect(kvbuilder.IsGPUResourceClaimTemplateName(vmName, otherName)).To(BeTrue())
		template := buildGPUResourceClaimTemplate(otherVM, otherName, buildGPUResourceClaimTemplateSpec(v1alpha2.GPUDeviceSpec{Name: "gpu0", GPUClassName: deviceClass}))
		fakeClient, _, vmState := setupEnvironment(vm, template)
		handler := NewGPUResourceClaimHandler(fakeClient)

		_, err := handler.Handle(context.Background(), vmState)

		Expect(err).NotTo(HaveOccurred())
		stored := &resourcev1.ResourceClaimTemplate{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: otherName, Namespace: namespace}, stored)).To(Succeed())
	})
})
