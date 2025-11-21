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

package operation

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	vmsopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
)

var _ = Describe("CreateVirtualMachineOperation", func() {
	const (
		name      = "test"
		namespace = "default"
	)

	var (
		ctx    context.Context
		secret *corev1.Secret
		vmsop  *v1alpha2.VirtualMachineSnapshotOperation
		vms    *v1alpha2.VirtualMachineSnapshot
	)

	newVMSOP := func(opts ...vmsopbuilder.Option) *v1alpha2.VirtualMachineSnapshotOperation {
		options := []vmsopbuilder.Option{
			vmsopbuilder.WithName(name),
			vmsopbuilder.WithNamespace(namespace),
			vmsopbuilder.WithCreateVirtualMachine(&v1alpha2.VMSOPCreateVirtualMachineSpec{
				Mode: v1alpha2.SnapshotOperationModeDryRun,
				Customization: &v1alpha2.VMSOPCreateVirtualMachineCustomization{
					NamePrefix: "prefix",
					NameSuffix: "suffix",
				},
			}),
		}
		options = append(options, opts...)
		return vmsopbuilder.New(options...)
	}

	newVMS := func(opts ...vmsnapshotbuilder.Option) *v1alpha2.VirtualMachineSnapshot {
		options := []vmsnapshotbuilder.Option{
			vmsnapshotbuilder.WithName("snapshot"),
			vmsnapshotbuilder.WithNamespace(namespace),
			vmsnapshotbuilder.WithVirtualMachineName("vm"),
			vmsnapshotbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressNever),
			vmsnapshotbuilder.WithRequiredConsistency(false),
		}
		options = append(options, opts...)
		return vmsnapshotbuilder.New(options...)
	}

	newSecret := func(name string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
	}

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		secret = newSecret("secret")
		vms = newVMS(vmsnapshotbuilder.WithVirtualMachineSnapshotSecretName(secret.Name))
		vmsop = newVMSOP(vmsopbuilder.WithVirtualMachineSnapshotName(vms.Name))
	})

	Describe("Execute", func() {
		It("should fail when CreateVirtualMachine spec is nil", func() {
			vmsop.Spec.CreateVirtualMachine = nil
			fakeClient, err := testutil.NewFakeClientWithObjects(vmsop, vms)
			Expect(err).NotTo(HaveOccurred())

			op := NewCreateVirtualMachineOperation(fakeClient, nil)

			_, execErr := op.Execute(ctx, vmsop)
			Expect(execErr).To(HaveOccurred())

			cond, exists := conditions.GetCondition(vmsopcondition.TypeCreateVirtualMachineCompleted, vmsop.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(vmsopcondition.ReasonCreateVirtualMachineOperationFailed)))
		})

		It("should return error when virtual machine snapshot is not found", func() {
			fakeClient, err := testutil.NewFakeClientWithObjects(vmsop)
			Expect(err).NotTo(HaveOccurred())

			op := NewCreateVirtualMachineOperation(fakeClient, nil)

			_, execErr := op.Execute(ctx, vmsop)
			Expect(execErr).To(HaveOccurred())
			Expect(execErr.Error()).To(ContainSubstring("specified virtual machine snapshot is not found"))
		})

		It("should return error when restorer secret is not found", func() {
			fakeClient, err := testutil.NewFakeClientWithObjects(vmsop, vms)
			Expect(err).NotTo(HaveOccurred())

			op := NewCreateVirtualMachineOperation(fakeClient, nil)

			_, execErr := op.Execute(ctx, vmsop)
			Expect(execErr).To(HaveOccurred())
			Expect(execErr.Error()).To(ContainSubstring("restorer secret \"secret\" is not found"))
		})
	})

	Describe("IsInProgress", func() {
		It("should return false when condition is missing", func() {
			op := CreateVirtualMachineOperation{}

			inProgress := op.IsInProgress(vmsop)
			Expect(inProgress).To(BeFalse())
		})

		It("should return false when condition status is Unknown", func() {
			vmsop.Status.Conditions = append(vmsop.Status.Conditions, metav1.Condition{
				Type:   vmsopcondition.TypeCreateVirtualMachineCompleted.String(),
				Status: metav1.ConditionUnknown,
			})

			op := CreateVirtualMachineOperation{}

			inProgress := op.IsInProgress(vmsop)
			Expect(inProgress).To(BeFalse())
		})

		It("should return true when condition status is not Unknown", func() {
			vmsop.Status.Conditions = append(vmsop.Status.Conditions, metav1.Condition{
				Type:   vmsopcondition.TypeCreateVirtualMachineCompleted.String(),
				Status: metav1.ConditionTrue,
			})

			op := CreateVirtualMachineOperation{}

			inProgress := op.IsInProgress(vmsop)
			Expect(inProgress).To(BeTrue())
		})
	})

	Describe("IsComplete", func() {
		It("should return not complete when condition is missing", func() {
			op := CreateVirtualMachineOperation{}

			complete, msg := op.IsFinished(vmsop)
			Expect(complete).To(BeFalse())
			Expect(msg).To(BeEmpty())
		})

		It("should return complete with message when operation failed", func() {
			vmsop := newVMSOP()
			vmsop.Status.Conditions = append(vmsop.Status.Conditions, metav1.Condition{
				Type:    vmsopcondition.TypeCreateVirtualMachineCompleted.String(),
				Status:  metav1.ConditionFalse,
				Reason:  string(vmsopcondition.ReasonCreateVirtualMachineOperationFailed),
				Message: "failure message",
			})

			op := CreateVirtualMachineOperation{}

			complete, msg := op.IsFinished(vmsop)
			Expect(complete).To(BeTrue())
			Expect(msg).To(Equal("failure message"))
		})

		It("should return complete without message when condition status is True", func() {
			vmsop := newVMSOP()
			vmsop.Status.Conditions = append(vmsop.Status.Conditions, metav1.Condition{
				Type:   vmsopcondition.TypeCreateVirtualMachineCompleted.String(),
				Status: metav1.ConditionTrue,
			})

			op := CreateVirtualMachineOperation{}

			complete, msg := op.IsFinished(vmsop)
			Expect(complete).To(BeTrue())
			Expect(msg).To(BeEmpty())
		})
	})
})
