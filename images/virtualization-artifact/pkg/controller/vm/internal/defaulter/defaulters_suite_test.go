/*
Copyright 2025 Flant JSC

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

package defaulter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/defaulter"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestDefaulters(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Defaulters Suite")
}

var _ = Describe("Set default class in virtualMachineClasName", func() {
	var (
		ctx            = testutil.ContextBackgroundWithNoOpLogger()
		classDefaulter *defaulter.VirtualMachineClassNameDefaulter
	)

	setup := func(objs ...client.Object) {
		GinkgoHelper()
		fakeClient, err := testutil.NewFakeClientWithObjects(objs...)
		Expect(err).Should(BeNil())
		vmClassService := service.NewVirtualMachineClassService(fakeClient)
		classDefaulter = defaulter.NewVirtualMachineClassNameDefaulter(fakeClient, vmClassService)
	}

	newVMClass := func(name string) *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineClassKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec:   v1alpha2.VirtualMachineClassSpec{},
			Status: v1alpha2.VirtualMachineClassStatus{},
		}
	}

	newDefaultVMClass := func(name string) *v1alpha2.VirtualMachineClass {
		vmClass := newVMClass(name)
		vmClass.Annotations = map[string]string{
			annotations.AnnVirtualMachineClassDefault: "true",
		}
		return vmClass
	}

	newVMWithEmptyClass := func(name string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualMachineKind,
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec:   v1alpha2.VirtualMachineSpec{},
			Status: v1alpha2.VirtualMachineStatus{},
		}
	}

	newVM := func(name, className string) *v1alpha2.VirtualMachine {
		vm := newVMWithEmptyClass(name)
		vm.Spec.VirtualMachineClassName = className
		return vm
	}

	AfterEach(func() {
		classDefaulter = nil
	})

	Context("creating VM with empty virtualMachineClassName", func() {
		It("should keep virtualMachineClassName empty if no default class", func() {
			// Initialize fake client with some classes.
			name := "single-custom-class"
			setup(
				newVMClass("generic"),
				newVMClass(name),
			)

			vm := newVMWithEmptyClass("vm-with-empty-class")
			err := classDefaulter.Default(ctx, vm)
			Expect(err).Should(BeNil())
			Expect(vm.Spec.VirtualMachineClassName).Should(BeEmpty())
		})

		It("should set virtualMachineClassName if default class is present", func() {
			// Initialize fake client with existing non default class.
			className := "single-default-class"
			setup(
				newVMClass("generic"),
				newDefaultVMClass(className),
			)

			vm := newVMWithEmptyClass("vm-with-empty-class")
			err := classDefaulter.Default(ctx, vm)
			Expect(err).Should(BeNil())
			Expect(vm.Spec.VirtualMachineClassName).Should(Equal(className))
		})
	})

	Context("creating VM with virtualMachineClassName", func() {
		It("should not change virtualMachineClassName if no default class", func() {
			// Initialize fake client with some classes.
			name := "single-custom-class"
			setup(
				newVMClass("generic"),
				newVMClass(name),
			)

			vm := newVM("vm-with-empty-class", "generic")
			err := classDefaulter.Default(ctx, vm)
			Expect(err).Should(BeNil())
			Expect(vm.Spec.VirtualMachineClassName).Should(Equal("generic"))
		})

		It("should not change virtualMachineClassName if default class is present", func() {
			// Initialize fake client with existing non default class.
			className := "single-default-class"
			setup(
				newVMClass("generic"),
				newDefaultVMClass(className),
			)

			vm := newVM("vm-with-empty-class", "generic")
			err := classDefaulter.Default(ctx, vm)
			Expect(err).Should(BeNil())
			Expect(vm.Spec.VirtualMachineClassName).Should(Equal("generic"))
		})
	})
})
