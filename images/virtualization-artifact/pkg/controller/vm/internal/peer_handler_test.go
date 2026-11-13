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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("PeerHandler", func() {
	const (
		name      = "vm-moved"
		namespace = "default"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
		recorder   *eventrecord.EventRecorderLoggerMock
	)

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
	})

	movedVM := func() *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Spec.RunPolicy = v1alpha2.AlwaysOnPolicy

		return vm
	}

	movedKVVM := func() *virtv1.VirtualMachine {
		kvvm := newEmptyKVVM(name, namespace)
		kvvm.Spec.RunStrategy = ptr.To(virtv1.RunStrategyManual)
		kvvm.Annotations = map[string]string{
			annotations.AnnPeerMigrationVMOP: "move",
		}

		return kvvm
	}
	showingMove := func() *v1alpha2.VirtualMachine {
		vm := movedVM()
		vm.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeMovedToPeer.String(), Status: metav1.ConditionTrue}}

		return vm
	}

	reconcile := func() {
		GinkgoHelper()

		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...any) {},
		}
		h := NewPeerHandler(fakeClient, fakeClient, recorder)
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.Update(context.Background())).To(Succeed())
	}

	It("keeps the lock while the operation that moves the VM exists", func() {
		vm := movedVM()
		kvvm := movedKVVM()
		op := vmopbuilder.New(
			vmopbuilder.WithName("move"),
			vmopbuilder.WithNamespace(namespace),
			vmopbuilder.WithVirtualMachine(name),
			vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
		)
		fakeClient, resource, vmState = setupEnvironment(vm, kvvm, op)

		reconcile()

		var gotKVVM virtv1.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), &gotKVVM)).To(Succeed())
		Expect(gotKVVM.Annotations).To(HaveKeyWithValue(annotations.AnnPeerMigrationVMOP, "move"))
		Expect(*gotKVVM.Spec.RunStrategy).To(Equal(virtv1.RunStrategyManual))
	})

	It("preserves AlwaysOn policy when the migration operation is gone and the guest is still running locally", func() {
		vm := movedVM()
		kvvm := movedKVVM()
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.Phase = virtv1.Running
		fakeClient, resource, vmState = setupEnvironment(vm, kvvm, kvvmi)

		reconcile()

		var gotKVVM virtv1.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), &gotKVVM)).To(Succeed())
		Expect(gotKVVM.Annotations).NotTo(HaveKey(annotations.AnnPeerMigrationVMOP))

		var gotVM v1alpha2.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), &gotVM)).To(Succeed())
		Expect(gotVM.Spec.RunPolicy).To(Equal(v1alpha2.AlwaysOnPolicy), "releasing a locally running VM must preserve its run policy")
	})

	It("releases a running VM with its AlwaysOn policy only after migration cancellation finishes", func() {
		vm := movedVM()
		kvvm := movedKVVM()
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.Phase = virtv1.Running
		migration := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "vmop-move", Finalizers: []string{"test/abort-pending"}},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationRunning},
		}
		fakeClient, resource, vmState = setupEnvironment(vm, kvvm, kvvmi, migration)

		reconcile()

		var gotKVVM virtv1.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), &gotKVVM)).To(Succeed())
		Expect(gotKVVM.Annotations).To(HaveKey(annotations.AnnPeerMigrationVMOP))
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(migration), migration)).To(Succeed())
		Expect(migration.DeletionTimestamp).NotTo(BeNil())
		migration.Status.Phase = virtv1.MigrationFailed
		Expect(fakeClient.Status().Update(ctx, migration)).To(Succeed())
		reconcile()
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), &gotKVVM)).To(Succeed())
		Expect(gotKVVM.Annotations).NotTo(HaveKey(annotations.AnnPeerMigrationVMOP))

		var gotVM v1alpha2.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), &gotVM)).To(Succeed())
		Expect(gotVM.Spec.RunPolicy).To(Equal(v1alpha2.AlwaysOnPolicy), "canceling migration of a locally running VM must preserve its run policy")
	})

	It("hands back an orphaned succeeded migration with Manual policy even if its VMI still shows Running", func() {
		vm := movedVM()
		kvvm := movedKVVM()
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.Phase = virtv1.Running
		migration := &virtv1.VirtualMachineInstanceMigration{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "vmop-move"},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationSucceeded},
		}
		fakeClient, resource, vmState = setupEnvironment(vm, kvvm, kvvmi, migration)

		reconcile()

		var gotVM v1alpha2.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), &gotVM)).To(Succeed())
		Expect(gotVM.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
	})

	// The guest may run in the peer: the run strategy from before the move would start a second copy here.
	It("releases the VM whose operation is gone stopped when the guest does not run here", func() {
		vm := movedVM()
		kvvm := movedKVVM()
		fakeClient, resource, vmState = setupEnvironment(vm, kvvm)

		reconcile()

		var gotKVVM virtv1.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), &gotKVVM)).To(Succeed())
		Expect(gotKVVM.Annotations).NotTo(HaveKey(annotations.AnnPeerMigrationVMOP))
		Expect(gotKVVM.Spec.RunStrategy).To(Equal(ptr.To(virtv1.RunStrategyManual)))

		var gotVM v1alpha2.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), &gotVM)).To(Succeed())
		Expect(gotVM.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
	})

	It("releases the VM whose operation is gone stopped when its guest has left", func() {
		vm := movedVM()
		kvvm := movedKVVM()
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.Phase = virtv1.Succeeded
		fakeClient, resource, vmState = setupEnvironment(vm, kvvm, kvvmi)

		reconcile()

		var gotKVVM virtv1.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), &gotKVVM)).To(Succeed())
		Expect(gotKVVM.Spec.RunStrategy).To(Equal(ptr.To(virtv1.RunStrategyManual)))

		var gotVM v1alpha2.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), &gotVM)).To(Succeed())
		Expect(gotVM.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
	})

	// The KubeVirt VM may not be seen: the condition locks the VM while an operation still moves it.
	It("keeps the lock of a VM whose KubeVirt VM is not seen while an operation moves it", func() {
		vm := showingMove()
		op := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "move", Namespace: namespace},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type: v1alpha2.VMOPTypeMigrate, VirtualMachine: name,
				Migrate: &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: "cluster-b"},
			},
			Status: v1alpha2.VirtualMachineOperationStatus{Phase: v1alpha2.VMOPPhaseInProgress},
		}
		fakeClient, resource, vmState = setupEnvironment(vm, op)

		reconcile()

		var got v1alpha2.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), &got)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(got.Status.Conditions, vmcondition.TypeMovedToPeer.String())).To(BeTrue())
	})

	It("releases a VM whose KubeVirt VM is not seen once no operation moves it", func() {
		vm := showingMove()
		fakeClient, resource, vmState = setupEnvironment(vm)

		reconcile()

		var got v1alpha2.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), &got)).To(Succeed())
		Expect(meta.FindStatusCondition(got.Status.Conditions, vmcondition.TypeMovedToPeer.String())).To(BeNil())
		// No guest runs here without the KubeVirt VM: it may run in the peer, and a new KubeVirt VM started
		// by the old run policy would be a second copy.
		Expect(got.Spec.RunPolicy).To(Equal(v1alpha2.ManualPolicy))
	})

	It("leaves a VM that does not move alone", func() {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Annotations = map[string]string{"another": "annotation"}
		fakeClient, resource, vmState = setupEnvironment(vm)

		reconcile()

		var got v1alpha2.VirtualMachine
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), &got)).To(Succeed())
		Expect(got.Annotations).To(HaveKeyWithValue("another", "annotation"))
	})
})

var _ = Describe("PeerHandler events", func() {
	It("tells the user that the operation is gone", func() {
		const ns = "default"

		vm := vmbuilder.NewEmpty("vm-moved", ns)
		kvvm := newEmptyKVVM("vm-moved", ns)
		kvvm.Annotations = map[string]string{annotations.AnnPeerMigrationVMOP: "move"}
		fakeClient, resource, vmState := setupEnvironment(vm, kvvm)

		var events []string

		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, reason, message string) {
				events = append(events, reason+": "+message)
			},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...any) {},
		}
		h := NewPeerHandler(fakeClient, fakeClient, recorder)
		_, err := h.Handle(testutil.ContextBackgroundWithNoOpLogger(), vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.Update(context.Background())).To(Succeed())

		Expect(events).To(HaveLen(1))
		Expect(events[0]).To(ContainSubstring(`The VirtualMachineOperation "move" that moved the virtual machine is gone`))
		Expect(events[0]).To(ContainSubstring("check the peer cluster"))
	})
})
