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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("DeletionHandler", func() {
	const (
		name      = "test"
		namespace = "default"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		srv        *reconciler.Resource[*virtv2.VirtualMachineOperation, virtv2.VirtualMachineOperationStatus]
	)

	AfterEach(func() {
		fakeClient = nil
		srv = nil
	})

	reconcile := func() {
		h := NewDeletionHandler(NewSvcOpCreator(fakeClient))
		_, err := h.Handle(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())
		err = srv.Update(ctx)
		Expect(err).NotTo(HaveOccurred())
	}

	newVmop := func(phase virtv2.VMOPPhase, opts ...vmopbuilder.Option) *virtv2.VirtualMachineOperation {
		vmop := vmopbuilder.NewEmpty(name, namespace)
		vmop.Status.Phase = phase
		vmopbuilder.ApplyOptions(vmop, opts)
		return vmop
	}

	DescribeTable("Should be protected", func(vmop *virtv2.VirtualMachineOperation, protect bool) {
		fakeClient, srv = setupEnvironment(vmop)
		reconcile()

		newVMOP := &virtv2.VirtualMachineOperation{}
		err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vmop), newVMOP)
		Expect(err).NotTo(HaveOccurred())

		updated := controllerutil.AddFinalizer(newVMOP, virtv2.FinalizerVMOPCleanup)

		if protect {
			Expect(updated).To(BeFalse())
		} else {
			Expect(updated).To(BeTrue())
		}
	},
		Entry("VMOP Start 1",
			newVmop(virtv2.VMOPPhaseInProgress, vmopbuilder.WithType(virtv2.VMOPTypeStart)),
			true,
		),
		Entry("VMOP Start 2",
			newVmop(virtv2.VMOPPhasePending, vmopbuilder.WithType(virtv2.VMOPTypeStart)),
			false,
		),
		Entry("VMOP Stop 1",
			newVmop(virtv2.VMOPPhaseInProgress, vmopbuilder.WithType(virtv2.VMOPTypeStop)),
			true,
		),
		Entry("VMOP Stop 2",
			newVmop(virtv2.VMOPPhaseCompleted, vmopbuilder.WithType(virtv2.VMOPTypeStop)),
			false,
		),
		Entry("VMOP Restart 1",
			newVmop(virtv2.VMOPPhaseInProgress, vmopbuilder.WithType(virtv2.VMOPTypeRestart)),
			true,
		),
		Entry("VMOP Restart 2",
			newVmop(virtv2.VMOPPhaseFailed, vmopbuilder.WithType(virtv2.VMOPTypeRestart)),
			false,
		),
		Entry("VMOP Evict 1",
			newVmop(virtv2.VMOPPhaseInProgress, vmopbuilder.WithType(virtv2.VMOPTypeEvict)),
			true,
		),
		Entry("VMOP Evict 2",
			newVmop(virtv2.VMOPPhasePending, vmopbuilder.WithType(virtv2.VMOPTypeEvict)),
			true,
		),
		Entry("VMOP Evict 3",
			newVmop(virtv2.VMOPPhaseCompleted, vmopbuilder.WithType(virtv2.VMOPTypeEvict)),
			true,
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
			fakeClient, srv = setupEnvironment(vmop, mig)
			reconcile()

			migs := &virtv1.VirtualMachineInstanceMigrationList{}
			err := fakeClient.List(ctx, migs)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(migs.Items)).To(Equal(expectLength))
		},
			Entry("VMOP Evict 1",
				newVmop(virtv2.VMOPPhaseInProgress, vmopbuilder.WithType(virtv2.VMOPTypeEvict), vmopbuilder.WithVirtualMachine("test-vm")),
				newSimpleMigration("vmop-"+name, namespace, "test-vm"), true,
			),
			Entry("VMOP Evict 2",
				newVmop(virtv2.VMOPPhaseCompleted, vmopbuilder.WithType(virtv2.VMOPTypeEvict), vmopbuilder.WithVirtualMachine("test-vm")),
				newSimpleMigration("vmop-"+name, namespace, "test-vm"), false,
			),
		)
	})
})
