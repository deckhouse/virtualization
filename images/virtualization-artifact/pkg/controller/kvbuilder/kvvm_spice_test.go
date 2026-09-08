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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Turning SPICE off is the interesting half: the builder is handed the KVVM that
// already exists, so anything the enabled branch wrote stays there unless it is
// removed explicitly.
var _ = Describe("SPICE devices", func() {
	name := types.NamespacedName{Name: "vm-spice", Namespace: "default"}

	newVM := func(spice *v1alpha2.SpiceSpec, videoAnnotation string) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Spice: spice}}
		if videoAnnotation != "" {
			vm.ObjectMeta = metav1.ObjectMeta{Annotations: map[string]string{annotations.AnnVideo: videoAnnotation}}
		}
		return vm
	}

	// The order matches ApplyVirtualMachineSpec: SetMetadata first, so a user-supplied
	// annotation is already on the template by the time the SPICE state is decided;
	// then the video model, so its own annotation wins over both branches.
	applyTo := func(kvvm *KVVM, vm *v1alpha2.VirtualMachine) *virtv1.VirtualMachine {
		kvvm.SetMetadata(vm.ObjectMeta)
		kvvm.SetSpiceDevices(vm)
		kvvm.SetVideoModel(vm)
		return kvvm.GetResource()
	}

	enabled := func() *virtv1.VirtualMachine {
		return applyTo(NewEmptyKVVM(name, KVVMOptions{}), newVM(&v1alpha2.SpiceSpec{Enabled: true}, ""))
	}

	It("attaches the display, the sound card and the redirection slots when enabled", func() {
		res := enabled()

		Expect(res.Spec.Template.Spec.Domain.Devices.Video).To(Equal(&virtv1.VideoDevice{Type: "virtio"}))
		Expect(res.Spec.Template.Spec.Domain.Devices.Sound).To(Equal(&virtv1.SoundDevice{Name: "sound0", Model: "ich9"}))
		Expect(res.Spec.Template.Spec.Domain.Devices.ClientPassthrough).ToNot(BeNil())
		Expect(res.Spec.Template.ObjectMeta.Annotations).To(HaveKeyWithValue(annotations.AnnSpice, "true"))
	})

	DescribeTable("detaches everything when it is turned off on an existing VirtualMachine",
		func(spice *v1alpha2.SpiceSpec) {
			res := applyTo(NewKVVM(enabled().DeepCopy(), KVVMOptions{}), newVM(spice, ""))

			Expect(res.Spec.Template.Spec.Domain.Devices.Video).To(BeNil())
			Expect(res.Spec.Template.Spec.Domain.Devices.Sound).To(BeNil())
			Expect(res.Spec.Template.Spec.Domain.Devices.ClientPassthrough).To(BeNil())
			Expect(res.Spec.Template.ObjectMeta.Annotations).ToNot(HaveKey(annotations.AnnSpice))
		},
		Entry("spice.enabled is false", &v1alpha2.SpiceSpec{Enabled: false}),
		Entry("the whole spice section is gone", nil),
	)

	It("keeps the video model the annotation asks for after SPICE is turned off", func() {
		res := applyTo(NewKVVM(enabled().DeepCopy(), KVVMOptions{}), newVM(&v1alpha2.SpiceSpec{Enabled: false}, "vga"))

		Expect(res.Spec.Template.Spec.Domain.Devices.Video).To(Equal(&virtv1.VideoDevice{Type: "vga"}))
		Expect(res.Spec.Template.Spec.Domain.Devices.Sound).To(BeNil())
		Expect(res.Spec.Template.ObjectMeta.Annotations).ToNot(HaveKey(annotations.AnnSpice))
	})

	// The annotation is how the platform talks to the fork, not a knob: a user editing
	// their own VirtualMachine must not be able to turn SPICE on with it, nor to claim
	// the memory overhead that comes with it. Goes through ApplyVirtualMachineSpec
	// rather than the calls above, because what protects it is their order there:
	// SetMetadata copies the annotations of the VirtualMachine onto the template, and
	// SetSpiceDevices has to run after it.
	It("ignores the annotation a user put on their own VirtualMachine", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name.Name,
				Namespace:   name.Namespace,
				Annotations: map[string]string{annotations.AnnSpice: "true"},
			},
			Spec: v1alpha2.VirtualMachineSpec{
				RunPolicy:  v1alpha2.ManualPolicy,
				OsType:     v1alpha2.GenericOs,
				Bootloader: v1alpha2.BIOS,
			},
		}
		class := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{CPU: v1alpha2.CPU{Type: v1alpha2.CPUTypeHost}},
		}

		kvvm := NewEmptyKVVM(name, KVVMOptions{})
		Expect(ApplyVirtualMachineSpec(kvvm, vm, nil, nil, nil, nil, class, "", nil, nil)).To(Succeed())
		res := kvvm.GetResource()

		Expect(res.Spec.Template.ObjectMeta.Annotations).ToNot(HaveKey(annotations.AnnSpice))
		Expect(res.Spec.Template.Spec.Domain.Devices.Video).To(BeNil())
		Expect(res.Spec.Template.Spec.Domain.Devices.Sound).To(BeNil())
		Expect(res.Spec.Template.Spec.Domain.Devices.ClientPassthrough).To(BeNil())
	})

	It("touches nothing on a VirtualMachine that never had SPICE", func() {
		res := applyTo(NewEmptyKVVM(name, KVVMOptions{}), newVM(nil, ""))

		Expect(res.Spec.Template.Spec.Domain.Devices.Video).To(BeNil())
		Expect(res.Spec.Template.Spec.Domain.Devices.Sound).To(BeNil())
		Expect(res.Spec.Template.Spec.Domain.Devices.ClientPassthrough).To(BeNil())
		Expect(res.Spec.Template.ObjectMeta.Annotations).ToNot(HaveKey(annotations.AnnSpice))
	})
})
