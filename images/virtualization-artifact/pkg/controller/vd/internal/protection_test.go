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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("The protection handler test", func() {
	const vdProtection = "virtualization.deckhouse.io/vd-protection"

	var (
		schema  *runtime.Scheme
		ctx     context.Context
		handler *ProtectionHandler
	)

	BeforeEach(func() {
		schema = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(schema)).To(Succeed())
		Expect(v1alpha2.AddToScheme(schema)).To(Succeed())
		Expect(virtv1.AddToScheme(schema)).To(Succeed())

		ctx = context.TODO()
	})

	Context("`VirtualDisk`", func() {
		When("has the `AttachedToVirtualMachines` status with the `Mounted` false value", func() {
			It("should remove the `vd-protection` finalizer from the `VirtualDisk`", func() {
				vd := &v1alpha2.VirtualDisk{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-virtual-disk",
						Namespace: "default",
						Finalizers: []string{
							vdProtection,
						},
					},
					Status: v1alpha2.VirtualDiskStatus{
						Conditions: []metav1.Condition{},
						AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
							{
								Name:    "test-virtual-machine",
								Mounted: false,
							},
						},
					},
				}

				handler = &ProtectionHandler{}
				result, err := handler.Handle(ctx, vd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsZero()).To(BeTrue())
				Expect(vd.Finalizers).NotTo(ContainElement(vdProtection))
			})
		})

		When("has the `AttachedToVirtualMachines` status with the `Mounted` true value", func() {
			It("should not remove the `vd-protection` finalizer from the `VirtualDisk`", func() {
				vd := &v1alpha2.VirtualDisk{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-virtual-disk",
						Namespace: "default",
						Finalizers: []string{
							vdProtection,
						},
					},
					Status: v1alpha2.VirtualDiskStatus{
						Conditions: []metav1.Condition{},
						AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
							{
								Name:    "test-virtual-machine",
								Mounted: true,
							},
						},
					},
				}

				handler = &ProtectionHandler{}
				result, err := handler.Handle(ctx, vd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsZero()).To(BeTrue())
				Expect(vd.Finalizers).To(ContainElement(vdProtection))
			})
		})
	})
})
