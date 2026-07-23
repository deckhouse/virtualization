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

package vmpool

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("pool validation webhook", func() {
	newValidator := func() *poolValidator {
		c, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())
		return &poolValidator{
			vmValidator:   vm.NewTemplateSpecValidator(c, featuregates.Default(), log.NewNop(), nil),
			diskValidator: vd.NewTemplateSpecValidator(c),
		}
	}

	Describe("vmFromTemplate", func() {
		It("maps the pool template onto a VirtualMachine", func() {
			pool := &v1alpha2.VirtualMachinePool{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns"},
				Spec: v1alpha2.VirtualMachinePoolSpec{
					VirtualMachineTemplate: v1alpha2.VirtualMachineTemplateSpec{
						Metadata: v1alpha2.VirtualMachineTemplateMetadata{
							Labels:      map[string]string{"app": "web"},
							Annotations: map[string]string{"team": "core"},
						},
						Spec: v1alpha2.VirtualMachineSpec{VirtualMachineClassName: "generic"},
					},
				},
			}
			vmObj := vmFromTemplate(pool)
			Expect(vmObj.GetNamespace()).To(Equal("ns"))
			Expect(vmObj.GetGenerateName()).To(Equal("web-"))
			Expect(vmObj.GetLabels()).To(HaveKeyWithValue("app", "web"))
			Expect(vmObj.GetAnnotations()).To(HaveKeyWithValue("team", "core"))
			Expect(vmObj.Spec.VirtualMachineClassName).To(Equal("generic"))
		})
	})

	Describe("ValidateCreate", func() {
		It("rejects a template with duplicate block device references", func() {
			pool := &v1alpha2.VirtualMachinePool{
				ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns"},
				Spec: v1alpha2.VirtualMachinePoolSpec{
					VirtualMachineTemplate: v1alpha2.VirtualMachineTemplateSpec{
						Spec: v1alpha2.VirtualMachineSpec{
							BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
								{Kind: v1alpha2.DiskDevice, Name: "root"},
								{Kind: v1alpha2.DiskDevice, Name: "root"},
							},
						},
					},
				},
			}
			_, err := newValidator().ValidateCreate(context.Background(), pool)
			Expect(err).To(HaveOccurred())
		})

		It("errors when the object is not a VirtualMachinePool", func() {
			_, err := (&poolValidator{}).ValidateCreate(context.Background(), &v1alpha2.VirtualMachine{})
			Expect(err).To(HaveOccurred())
		})
	})
})
