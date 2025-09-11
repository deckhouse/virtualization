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

package handler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("DeletionHandler", func() {
	const (
		name      = "test"
		namespace = "default"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		srv        *reconciler.Resource[*v1alpha2.VirtualMachineOperation, v1alpha2.VirtualMachineOperationStatus]
	)

	AfterEach(func() {
		fakeClient = nil
		srv = nil
	})

	reconcile := func() {
		h := NewDeletionHandler(fakeClient)
		_, err := h.Handle(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())
		err = srv.Update(ctx)
		Expect(err).NotTo(HaveOccurred())
	}

	newVmop := func(phase v1alpha2.VMOPPhase, opts ...vmopbuilder.Option) *v1alpha2.VirtualMachineOperation {
		vmop := vmopbuilder.NewEmpty(name, namespace)
		vmop.Status.Phase = phase
		vmop.Status = v1alpha2.VirtualMachineOperationStatus{Conditions: []metav1.Condition{}}
		vmop.Spec.VirtualMachine = "test-vm"
		vmopbuilder.ApplyOptions(vmop, opts...)
		return vmop
	}

	DescribeTable("Should be protected", func(phase v1alpha2.VMOPPhase, protect bool) {
		vmop := newVmop(phase, vmopbuilder.WithType(v1alpha2.VMOPTypeRestore))

		fakeClient, srv = setupEnvironment(vmop)
		reconcile()

		newVMOP := &v1alpha2.VirtualMachineOperation{}
		err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vmop), newVMOP)
		Expect(err).NotTo(HaveOccurred())

		updated := controllerutil.AddFinalizer(newVMOP, v1alpha2.FinalizerVMOPCleanup)

		if protect {
			Expect(updated).To(BeFalse())
		} else {
			Expect(updated).To(BeTrue())
		}
	},
		Entry("VMOP Restore 1", v1alpha2.VMOPPhasePending, false),
		Entry("VMOP Restore 2", v1alpha2.VMOPPhaseInProgress, false),
		Entry("VMOP Restore 3", v1alpha2.VMOPPhaseCompleted, false),
		Entry("VMOP Restore 4", v1alpha2.VMOPPhaseFailed, false),
	)
})
