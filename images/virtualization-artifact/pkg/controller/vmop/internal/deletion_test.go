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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/builder"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("DeletionHandler", func() {
	var (
		fakeClient client.WithWatch
		srv        *reconciler.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
		s          state.VMOperationState
	)
	AfterEach(func() {
		fakeClient = nil
		srv = nil
		s = nil
	})

	reconcile := func() {
		vmopSrv := service.NewVMOperationService(fakeClient)
		h := NewDeletionHandler(vmopSrv)
		_, err := h.Handle(testutil.ContextBackgroundWithNoOpLogger(), s)
		Expect(err).NotTo(HaveOccurred())
		err = srv.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	DescribeTable("Should be protected", func(vmop *virtv2.VirtualMachineOperation, protect bool) {
		fakeClient, srv, s = setupEnvironment(vmop)
		reconcile()

		key := types.NamespacedName{
			Name:      vmop.GetName(),
			Namespace: vmop.GetNamespace(),
		}

		newVMOP := &virtv2.VirtualMachineOperation{}
		err := fakeClient.Get(context.Background(), key, newVMOP)
		Expect(err).NotTo(HaveOccurred())

		updated := controllerutil.AddFinalizer(newVMOP, virtv2.FinalizerVMOPCleanup)

		if protect {
			Expect(updated).To(BeFalse())
		} else {
			Expect(updated).To(BeTrue())
		}
	},
		Entry("VMOP Start 1",
			builder.NewVMOPBuilder("test-vmop-start-1", "test").
				WithType(virtv2.VMOPTypeStart).
				WithStatusPhase(virtv2.VMOPPhaseInProgress).
				Complete(),
			true,
		),
		Entry("VMOP Start 2",
			builder.NewVMOPBuilder("test-vmop-start-2", "test").
				WithType(virtv2.VMOPTypeStart).
				WithStatusPhase(virtv2.VMOPPhasePending).
				Complete(),
			false,
		),
		Entry("VMOP Stop 1",
			builder.NewVMOPBuilder("test-vmop-stop-1", "test").
				WithType(virtv2.VMOPTypeStop).
				WithStatusPhase(virtv2.VMOPPhaseInProgress).
				Complete(),
			true,
		),
		Entry("VMOP Stop 2",
			builder.NewVMOPBuilder("test-vmop-stop-2", "test").
				WithType(virtv2.VMOPTypeStop).
				WithStatusPhase(virtv2.VMOPPhaseCompleted).
				Complete(),
			false,
		),
		Entry("VMOP Restart 1",
			builder.NewVMOPBuilder("test-vmop-restart-1", "test").
				WithType(virtv2.VMOPTypeRestart).
				WithStatusPhase(virtv2.VMOPPhaseInProgress).
				Complete(),
			true,
		),
		Entry("VMOP Restart 2",
			builder.NewVMOPBuilder("test-vmop-restart-2", "test").
				WithType(virtv2.VMOPTypeRestart).
				WithStatusPhase(virtv2.VMOPPhaseFailed).
				Complete(),
			false,
		),
		Entry("VMOP Evict 1",
			builder.NewVMOPBuilder("test-vmop-evict-1", "test").
				WithType(virtv2.VMOPTypeEvict).
				WithStatusPhase(virtv2.VMOPPhaseInProgress).
				Complete(), true,
		),
		Entry("VMOP Evict 2",
			builder.NewVMOPBuilder("test-vmop-evict-1", "test").
				WithType(virtv2.VMOPTypeEvict).
				WithStatusPhase(virtv2.VMOPPhasePending).
				Complete(), true,
		),
		Entry("VMOP Evict 3",
			builder.NewVMOPBuilder("test-vmop-evict-1", "test").
				WithType(virtv2.VMOPTypeEvict).
				WithStatusPhase(virtv2.VMOPPhaseCompleted).
				Complete(), true,
		),
	)

	Context("Migration", func() {
		DescribeTable("Should cleanup migration", func(vmop *virtv2.VirtualMachineOperation, mig *virtv1.VirtualMachineInstanceMigration, shouldExist bool) {
			expectLength := 1
			if !shouldExist {
				controllerutil.AddFinalizer(vmop, virtv2.FinalizerVMOPCleanup)
				vmop.DeletionTimestamp = ptr.To(metav1.Now())
				expectLength = 0
			}
			fakeClient, srv, s = setupEnvironment(vmop, mig)
			reconcile()

			migs := &virtv1.VirtualMachineInstanceMigrationList{}
			err := fakeClient.List(context.Background(), migs)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(migs.Items)).To(Equal(expectLength))
		},
			Entry("VMOP Evict 1",
				builder.NewVMOPBuilder("test-vmop-evict-1", "test").
					WithType(virtv2.VMOPTypeEvict).
					WithStatusPhase(virtv2.VMOPPhaseInProgress).
					Complete(),
				newSimpleMigration("vmop-test-vmop-evict-1", "test", "test-vm"), true,
			),
			Entry("VMOP Evict 2",
				builder.NewVMOPBuilder("test-vmop-evict-2", "test").
					WithType(virtv2.VMOPTypeEvict).
					WithStatusPhase(virtv2.VMOPPhaseCompleted).
					Complete(),
				newSimpleMigration("vmop-test-vmop-evict-2", "test", "test-vm"), false,
			),
		)
	})
})
