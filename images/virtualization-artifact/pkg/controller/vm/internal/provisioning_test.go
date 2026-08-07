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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("ProvisioningHandler", func() {
	const (
		name       = "vm-provisioning"
		namespace  = "default"
		secretName = "provisioning-data"
	)

	const validCloudConfig = "#cloud-config\nusers:\n  - name: cloud\n"
	// The shell entry is indented past its sibling, which cloud-init cannot parse.
	const brokenCloudConfig = "#cloud-config\nusers:\n - name: cloud\n   groups: sudo\n  shell: /bin/bash\n"

	var events []string

	newVM := func(provisioning *v1alpha2.Provisioning) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Spec.Provisioning = provisioning
		return vm
	}

	newSecret := func(secretType corev1.SecretType, data map[string][]byte) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			Type:       secretType,
			Data:       data,
		}
	}

	// reconcile runs the handler to completion: the first pass only seeds the
	// unknown condition and asks for a requeue.
	reconcile := func(vm *v1alpha2.VirtualMachine, objs ...client.Object) *metav1.Condition {
		GinkgoHelper()

		events = nil
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, reason, message string) {
				events = append(events, reason+": "+message)
			},
		}

		fakeClient, resource, vmState := setupEnvironment(vm, objs...)
		h := NewProvisioningHandler(fakeClient, recorder)

		_, err := h.Handle(testutil.ContextBackgroundWithNoOpLogger(), vmState)
		Expect(err).NotTo(HaveOccurred())
		_, err = h.Handle(testutil.ContextBackgroundWithNoOpLogger(), vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.Update(context.Background())).To(Succeed())

		updated := &v1alpha2.VirtualMachine{}
		Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(vm), updated)).To(Succeed())

		cond, found := conditions.GetCondition(vmcondition.TypeProvisioningReady, updated.Status.Conditions)
		Expect(found).To(BeTrue())
		return &cond
	}

	Context("when provisioning is not configured", func() {
		It("is ready", func() {
			cond := reconcile(newVM(nil))

			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(events).To(BeEmpty())
		})
	})

	Context("with inline user data", func() {
		DescribeTable("is ready and silent for user data cloud-init understands",
			func(userData string) {
				cond := reconcile(newVM(&v1alpha2.Provisioning{
					Type:     v1alpha2.ProvisioningTypeUserData,
					UserData: userData,
				}))

				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(cond.Reason).To(Equal(vmcondition.ReasonProvisioningReady.String()))
				Expect(cond.Message).To(BeEmpty())
				Expect(events).To(BeEmpty())
			},
			Entry("a cloud-config", validCloudConfig),
			Entry("a shell script", "#!/bin/bash\necho hello\n"),
			Entry("a boot hook", "#cloud-boothook\n#!/bin/sh\necho early\n"),
			Entry("an include file", "#include\nhttps://example.com/config\n"),
			Entry("a part handler", "#part-handler\ndef list_types():\n    pass\n"),
			Entry("a cloud-config archive", "#cloud-config-archive\n- type: text/cloud-config\n  content: |\n    hostname: vm\n"),
			Entry("a jsonp patch", "#cloud-config-jsonp\n[{\"op\": \"add\", \"path\": \"/hostname\", \"value\": \"vm\"}]\n"),
			Entry("a jinja template", "## template: jinja\n#cloud-config\nhostname: {{ v1.local_hostname }}\n"),
			Entry("a MIME archive", "MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=\"==B==\"\n"),
		)

		It("stays ready but reports a cloud-config the guest cannot parse", func() {
			cond := reconcile(newVM(&v1alpha2.Provisioning{
				Type:     v1alpha2.ProvisioningTypeUserData,
				UserData: brokenCloudConfig,
			}))

			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonProvisioningReadyWithWarnings.String()))
			Expect(cond.Message).To(ContainSubstring("not a valid cloud-config"))
			Expect(events).To(HaveLen(1))
			Expect(events[0]).To(HavePrefix("ProvisioningInvalid: "))
		})

		DescribeTable("stays ready but reports a jinja header cloud-init does not recognize",
			func(userData string) {
				cond := reconcile(newVM(&v1alpha2.Provisioning{
					Type:     v1alpha2.ProvisioningTypeUserData,
					UserData: userData,
				}))

				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(cond.Reason).To(Equal(vmcondition.ReasonProvisioningReadyWithWarnings.String()))
				Expect(cond.Message).To(ContainSubstring("## template: jinja"))
			},
			Entry("no space after the hashes", "##template: jinja\n#cloud-config\nhostname: vm\n"),
			Entry("no space after the colon", "## template:jinja\n#cloud-config\nhostname: vm\n"),
		)

		It("is not ready when user data is empty", func() {
			cond := reconcile(newVM(&v1alpha2.Provisioning{
				Type:     v1alpha2.ProvisioningTypeUserData,
				UserData: "",
			}))

			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("with a user data secret", func() {
		userDataRef := &v1alpha2.Provisioning{
			Type:        v1alpha2.ProvisioningTypeUserDataRef,
			UserDataRef: &v1alpha2.UserDataRef{Kind: v1alpha2.UserDataRefKindSecret, Name: secretName},
		}

		It("is ready and silent for a valid cloud-config", func() {
			secret := newSecret(v1alpha2.SecretTypeCloudInit, map[string][]byte{"userData": []byte(validCloudConfig)})

			cond := reconcile(newVM(userDataRef), secret)

			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Message).To(BeEmpty())
			Expect(events).To(BeEmpty())
		})

		It("reads the lowercase key as well", func() {
			secret := newSecret(v1alpha2.SecretTypeCloudInit, map[string][]byte{"userdata": []byte(validCloudConfig)})

			cond := reconcile(newVM(userDataRef), secret)

			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(events).To(BeEmpty())
		})

		It("stays ready but reports a cloud-config the guest cannot parse", func() {
			secret := newSecret(v1alpha2.SecretTypeCloudInit, map[string][]byte{"userData": []byte(brokenCloudConfig)})

			cond := reconcile(newVM(userDataRef), secret)

			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonProvisioningReadyWithWarnings.String()))
			Expect(cond.Message).To(ContainSubstring("not a valid cloud-config"))
			Expect(events).To(HaveLen(1))
		})

		It("stays ready but reports a missing cloud-init header", func() {
			secret := newSecret(v1alpha2.SecretTypeCloudInit, map[string][]byte{"userData": []byte("users: []\n")})

			cond := reconcile(newVM(userDataRef), secret)

			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Message).To(ContainSubstring("cloud-init will ignore it"))
		})

		It("is not ready when the secret carries no user data key", func() {
			secret := newSecret(v1alpha2.SecretTypeCloudInit, map[string][]byte{"something-else": []byte("x")})

			cond := reconcile(newVM(userDataRef), secret)

			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(events).To(BeEmpty())
		})

		It("is not ready when the secret is missing", func() {
			cond := reconcile(newVM(userDataRef))

			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Message).To(ContainSubstring("not found"))
		})

		It("is not ready when the secret has the wrong type", func() {
			secret := newSecret(corev1.SecretTypeOpaque, map[string][]byte{"userData": []byte(validCloudConfig)})

			cond := reconcile(newVM(userDataRef), secret)

			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Message).To(ContainSubstring("Unexpected secret type"))
		})
	})

	Context("with a sysprep secret", func() {
		sysprepRef := &v1alpha2.Provisioning{
			Type:       v1alpha2.ProvisioningTypeSysprepRef,
			SysprepRef: &v1alpha2.SysprepRef{Kind: v1alpha2.SysprepRefKindSecret, Name: secretName},
		}

		DescribeTable("is ready and silent when an answer file is present",
			func(key string) {
				secret := newSecret(v1alpha2.SecretTypeSysprep, map[string][]byte{key: []byte("<unattend/>")})

				cond := reconcile(newVM(sysprepRef), secret)

				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(cond.Message).To(BeEmpty())
				Expect(events).To(BeEmpty())
			},
			Entry("autounattend.xml", "autounattend.xml"),
			Entry("unattend.xml", "unattend.xml"),
		)

		It("stays ready but reports a secret with no answer file", func() {
			secret := newSecret(v1alpha2.SecretTypeSysprep, map[string][]byte{"readme.txt": []byte("x")})

			cond := reconcile(newVM(sysprepRef), secret)

			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonProvisioningReadyWithWarnings.String()))
			Expect(cond.Message).To(ContainSubstring("answer file"))
			Expect(events).To(HaveLen(1))
		})
	})
})
