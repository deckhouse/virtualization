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

	vmsopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsop"
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
		srv        *reconciler.Resource[*v1alpha2.VirtualMachineSnapshotOperation, v1alpha2.VirtualMachineSnapshotOperationStatus]
	)

	AfterEach(func() {
		fakeClient = nil
		srv = nil
	})

	reconcile := func() {
		h := NewDeletionHandler(fakeClient)
		_, err := h.Handle(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())
		err = fakeClient.Update(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())
		err = srv.Update(ctx)
		Expect(err).NotTo(HaveOccurred())
	}

	newVmsop := func(phase v1alpha2.VMSOPPhase, opts ...vmsopbuilder.Option) *v1alpha2.VirtualMachineSnapshotOperation {
		vmsop := vmsopbuilder.NewEmpty(name, namespace)
		vmsop.Status = v1alpha2.VirtualMachineSnapshotOperationStatus{
			Phase:      phase,
			Conditions: []metav1.Condition{},
		}
		vmsop.Spec.VirtualMachineSnapshot = "test-vm"
		vmsopbuilder.ApplyOptions(vmsop, opts...)
		return vmsop
	}

	DescribeTable("Should be protected", func(phase v1alpha2.VMSOPPhase, protect bool) {
		vmop := newVmsop(phase, vmsopbuilder.WithType(v1alpha2.VMSOPTypeClone))

		fakeClient, srv = setupEnvironment(vmop)
		reconcile()

		newVMSOP := &v1alpha2.VirtualMachineSnapshotOperation{}
		err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vmop), newVMSOP)
		Expect(err).NotTo(HaveOccurred())

		if protect {
			Expect(controllerutil.ContainsFinalizer(newVMSOP, v1alpha2.FinalizerVMOPCleanup)).To(BeTrue())
		} else {
			Expect(controllerutil.ContainsFinalizer(newVMSOP, v1alpha2.FinalizerVMOPCleanup)).To(BeFalse())
		}
	},
		Entry("VMSOP pending", v1alpha2.VMOPPhasePending, false),
		Entry("VMSOP in progress", v1alpha2.VMOPPhaseInProgress, true),
		Entry("VMSOP completed", v1alpha2.VMOPPhaseCompleted, false),
		Entry("VMSOP failed", v1alpha2.VMOPPhaseFailed, false),
	)
})
