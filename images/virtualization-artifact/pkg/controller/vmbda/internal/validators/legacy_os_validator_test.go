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

package validators_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("LegacyOSValidator (VMBDA)", func() {
	const ns = "test-ns"

	makeVM := func(osType v1alpha2.OsType, paravirt bool) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: ns},
			Spec: v1alpha2.VirtualMachineSpec{
				OsType:                   osType,
				EnableParavirtualization: ptr.To(paravirt),
			},
		}
	}

	vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "vmbda", Namespace: ns},
		Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
			VirtualMachineName: "vm",
			BlockDeviceRef: v1alpha2.VMBDAObjectRef{
				Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
				Name: "disk",
			},
		},
	}

	makeValidator := func(objs ...client.Object) *validators.LegacyOSValidator {
		fakeClient, err := testutil.NewFakeClientWithObjects(objs...)
		Expect(err).NotTo(HaveOccurred())
		return validators.NewLegacyOSValidator(service.NewAttachmentService(fakeClient, nil, ""))
	}

	It("rejects create for a Legacy VM without paravirtualization", func() {
		v := makeValidator(makeVM(v1alpha2.LegacyOs, false))
		_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("hot-plugging is not available"))
	})

	It("allows create for a Legacy VM with paravirtualization: the osType picks a chipset, not a guest", func() {
		v := makeValidator(makeVM(v1alpha2.LegacyOs, true))
		_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("allows create for other osTypes", func() {
		for _, osType := range []v1alpha2.OsType{v1alpha2.GenericOs, v1alpha2.Windows} {
			for _, paravirt := range []bool{true, false} {
				v := makeValidator(makeVM(osType, paravirt))
				_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
				Expect(err).ShouldNot(HaveOccurred())
			}
		}
	})

	It("allows create when the VM does not exist yet", func() {
		v := makeValidator()
		_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
		Expect(err).ShouldNot(HaveOccurred())
	})
})
