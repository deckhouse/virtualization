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

package validators

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("ProvisioningValidator", func() {
	newVM := func(provisioning *v1alpha2.Provisioning) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{Provisioning: provisioning},
		}
	}

	inlineVM := func(userData string) *v1alpha2.VirtualMachine {
		return newVM(&v1alpha2.Provisioning{
			Type:     v1alpha2.ProvisioningTypeUserData,
			UserData: userData,
		})
	}

	It("accepts a machine without provisioning", func() {
		warnings, err := NewProvisioningValidator().Validate(newVM(nil))

		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(BeEmpty())
	})

	It("accepts a valid cloud-config", func() {
		warnings, err := NewProvisioningValidator().Validate(inlineVM(
			"#cloud-config\nusers:\n  - name: cloud\n    sudo: ALL=(ALL) NOPASSWD:ALL\n",
		))

		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(BeEmpty())
	})

	It("warns instead of refusing a cloud-config cloud-init cannot parse", func() {
		warnings, err := NewProvisioningValidator().Validate(inlineVM(
			"#cloud-config\nusers:\n - name: cloud\n   groups: sudo\n  shell: /bin/bash\n",
		))

		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(ConsistOf(
			And(ContainSubstring("spec.provisioning.userData"), ContainSubstring("not a valid cloud-config")),
		))
	})

	It("warns instead of refusing when the header is missing", func() {
		warnings, err := NewProvisioningValidator().Validate(inlineVM("users:\n  - name: cloud\n"))

		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(HaveLen(1))
		Expect(warnings[0]).To(ContainSubstring("spec.provisioning.userData"))
	})

	It("warns instead of refusing when user data is empty", func() {
		warnings, err := NewProvisioningValidator().Validate(inlineVM(""))

		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(HaveLen(1))
		Expect(warnings[0]).To(ContainSubstring("empty"))
	})

	DescribeTable("leaves data kept outside the machine to the provisioning handler",
		func(provisioning *v1alpha2.Provisioning) {
			warnings, err := NewProvisioningValidator().Validate(newVM(provisioning))

			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		},
		Entry("a user data secret", &v1alpha2.Provisioning{
			Type:        v1alpha2.ProvisioningTypeUserDataRef,
			UserDataRef: &v1alpha2.UserDataRef{Kind: v1alpha2.UserDataRefKindSecret, Name: "cloud-init"},
		}),
		Entry("a sysprep secret", &v1alpha2.Provisioning{
			Type:       v1alpha2.ProvisioningTypeSysprepRef,
			SysprepRef: &v1alpha2.SysprepRef{Kind: v1alpha2.SysprepRefKindSecret, Name: "sysprep"},
		}),
	)

	It("checks the payload on update as well as on create", func() {
		broken := inlineVM("#cloud-config\nhostname: \"vm\n")

		warnings, err := NewProvisioningValidator().ValidateUpdate(context.TODO(), inlineVM("#cloud-config\n"), broken)

		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(ConsistOf(ContainSubstring("not a valid cloud-config")))
	})

	It("never refuses a payload, whatever is wrong with it", func() {
		for _, userData := range []string{
			"",
			"just some text\n",
			"#cloud-config\nhostname: \"vm\n",
			"#cloud-config-archive\nhostname: vm\n",
			string([]byte{0x00, 0x01, 0x02}),
		} {
			_, err := NewProvisioningValidator().ValidateCreate(context.TODO(), inlineVM(userData))

			Expect(err).NotTo(HaveOccurred(), "user data %q must not be refused", userData)
		}
	})
})
