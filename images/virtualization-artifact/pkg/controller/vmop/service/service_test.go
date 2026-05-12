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

package service

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func TestService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VMOP Service Suite")
}

var _ = Describe("BaseVMOPService", func() {
	Describe("ShouldExecuteOrSupersedeOrSetFailedPhase", func() {
		It("marks an allowed older operation as superseded", func(ctx SpecContext) {
			oldVMOP := newVMOP("old-start", v1alpha2.VMOPTypeStart, false, time.Now().Add(-time.Minute))
			newVMOP := newVMOP("new-stop", v1alpha2.VMOPTypeStop, false, time.Now())

			client, err := testutil.NewFakeClientWithObjects(oldVMOP)
			Expect(err).NotTo(HaveOccurred())

			svc := NewBaseVMOPService(client, &eventrecord.EventRecorderLoggerMock{})
			should, err := svc.ShouldExecuteOrSupersedeOrSetFailedPhase(ctx, newVMOP)
			Expect(err).NotTo(HaveOccurred())
			Expect(should).To(BeTrue())

			changed := &v1alpha2.VirtualMachineOperation{}
			Expect(client.Get(ctx, types.NamespacedName{Name: oldVMOP.Name, Namespace: oldVMOP.Namespace}, changed)).To(Succeed())
			Expect(changed.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))

			completed, found := conditions.GetCondition(vmopcondition.TypeCompleted, changed.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(completed.Status).To(Equal(metav1.ConditionTrue))
			Expect(completed.Reason).To(Equal(vmopcondition.ReasonSuperseded.String()))
		})

		It("fails a forbidden newer operation", func(ctx SpecContext) {
			oldVMOP := newVMOP("old-stop", v1alpha2.VMOPTypeStop, false, time.Now().Add(-time.Minute))
			newVMOP := newVMOP("new-start", v1alpha2.VMOPTypeStart, false, time.Now())

			client, err := testutil.NewFakeClientWithObjects(oldVMOP)
			Expect(err).NotTo(HaveOccurred())

			svc := NewBaseVMOPService(client, &eventrecord.EventRecorderLoggerMock{})
			should, err := svc.ShouldExecuteOrSupersedeOrSetFailedPhase(ctx, newVMOP)
			Expect(err).NotTo(HaveOccurred())
			Expect(should).To(BeFalse())
			Expect(newVMOP.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))

			completed, found := conditions.GetCondition(vmopcondition.TypeCompleted, newVMOP.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(completed.Status).To(Equal(metav1.ConditionFalse))
			Expect(completed.Reason).To(Equal(vmopcondition.ReasonNotReadyToBeExecuted.String()))
		})
	})
})

func newVMOP(name string, vmopType v1alpha2.VMOPType, force bool, createdAt time.Time) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			UID:               types.UID(name),
			CreationTimestamp: metav1.NewTime(createdAt),
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           vmopType,
			VirtualMachine: "test-vm",
			Force:          ptr.To(force),
		},
		Status: v1alpha2.VirtualMachineOperationStatus{
			Phase: v1alpha2.VMOPPhaseInProgress,
		},
	}
}
