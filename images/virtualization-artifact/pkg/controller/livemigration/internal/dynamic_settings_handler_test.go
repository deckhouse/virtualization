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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/livemigration"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TestDynamicSettingsHandler", func() {
	const (
		vmName      = "vm-migratable"
		vmNamespace = "default"
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	newVM := func() *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(vmName, vmNamespace)
		vm.Spec.LiveMigrationPolicy = v1alpha2.PreferSafeMigrationPolicy

		return vm
	}

	newKVVMI := func() *virtv1.VirtualMachineInstance {
		vmi := &virtv1.VirtualMachineInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: virtv1.SchemeGroupVersion.String(),
				Kind:       virtv1.VirtualMachineInstanceGroupVersionKind.Kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
			},
		}
		return vmi
	}

	withMigrationState := func(kvvmi *virtv1.VirtualMachineInstance, migrationUID string) {
		kvvmi.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{
			TargetNode:   "node-a",
			MigrationUID: types.UID(migrationUID),
		}
	}

	newVMOPEvict := func(force *bool) *v1alpha2.VirtualMachineOperation {
		vmop := &v1alpha2.VirtualMachineOperation{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineOperationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-vmop-name",
				Namespace: vmNamespace,
			},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type:           v1alpha2.VMOPTypeEvict,
				VirtualMachine: vmName,
				Force:          force,
			},
			Status: v1alpha2.VirtualMachineOperationStatus{
				Phase: v1alpha2.VMOPPhaseInProgress,
			},
		}
		return vmop
	}

	newKVConfig := func() *virtv1.KubeVirt {
		return &virtv1.KubeVirt{
			TypeMeta: metav1.TypeMeta{
				APIVersion: virtv1.SchemeGroupVersion.String(),
				Kind:       virtv1.KubeVirtGroupVersionKind.Kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config",
				Namespace: "d8-virtualization",
			},
			Spec:   virtv1.KubeVirtSpec{},
			Status: virtv1.KubeVirtStatus{},
		}
	}

	When("Observe KVVMI with migrateState", func() {
		It("Should set migrationConfiguration", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")

			fakeClient := setupEnvironment(kvvmi, vm, newKVConfig())
			h := NewDynamicSettingsHandler(fakeClient, livemigration.NewInboundMigrationLimiter(true, 1), livemigration.NewSyncMigrationLimiter(false, 1))
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.InboundMigrationSlotAnnotation, livemigration.InboundMigrationSlotAcquired))
		})

		It("Should wait without migrationConfiguration when inbound slot is busy", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")

			otherKVVMI := newKVVMI()
			otherKVVMI.Name = "other-vm"
			withMigrationState(otherKVVMI, "other-migration-uid")

			fakeClient := setupEnvironment(kvvmi, vm, otherKVVMI, newKVConfig())
			inboundLimiter := livemigration.NewInboundMigrationLimiter(true, 1)
			Expect(inboundLimiter.TryAcquire(otherKVVMI, "node-a")).To(BeTrue())

			h := NewDynamicSettingsHandler(fakeClient, inboundLimiter, livemigration.NewSyncMigrationLimiter(false, 1))
			res, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(res.RequeueAfter).To(BeNumerically(">", 0))
			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).Should(BeNil(), "Should not set migrationConfiguration")
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.InboundMigrationSlotAnnotation, livemigration.InboundMigrationSlotWaiting))
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.InboundMigrationTargetNodeAnnotation, "node-a"))
		})

		It("Should propagate DisableTLS from KubeVirt config", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")

			kvConfig := newKVConfig()
			kvConfig.Spec.Configuration.MigrationConfiguration = &virtv1.MigrationConfiguration{
				DisableTLS: ptr.To(true),
			}

			fakeClient := setupEnvironment(kvvmi, vm, kvConfig)
			h := NewDynamicSettingsHandler(fakeClient, livemigration.NewInboundMigrationLimiter(true, 1), livemigration.NewSyncMigrationLimiter(false, 1))
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
			Expect(kvvmi.Status.MigrationState.MigrationConfiguration.DisableTLS).ShouldNot(BeNil(), "Should propagate DisableTLS")
			Expect(*kvvmi.Status.MigrationState.MigrationConfiguration.DisableTLS).To(BeTrue())
		})
	})

	When("Sync migration limiter is enabled", func() {
		It("Should acquire a sync slot and set migrationConfiguration", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")
			kvvmi.Status.MigrationState.SourceNode = "node-src"

			fakeClient := setupEnvironment(kvvmi, vm, newKVConfig())
			h := NewDynamicSettingsHandler(fakeClient, livemigration.NewInboundMigrationLimiter(false, 1), livemigration.NewSyncMigrationLimiter(true, 1))
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.SyncMigrationSlotAnnotation, livemigration.SyncMigrationSlotAcquired))
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.SyncMigrationSourceNodeAnnotation, "node-src"))
		})

		It("Should wait without migrationConfiguration when the sync slot is busy", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")
			kvvmi.Status.MigrationState.SourceNode = "node-src"

			otherKVVMI := newKVVMI()
			otherKVVMI.Name = "other-vm"
			withMigrationState(otherKVVMI, "other-migration-uid")
			otherKVVMI.Status.MigrationState.SourceNode = "node-src"

			fakeClient := setupEnvironment(kvvmi, vm, otherKVVMI, newKVConfig())
			syncLimiter := livemigration.NewSyncMigrationLimiter(true, 1)
			Expect(syncLimiter.TryAcquire(otherKVVMI, "node-src")).To(BeTrue())

			h := NewDynamicSettingsHandler(fakeClient, livemigration.NewInboundMigrationLimiter(false, 1), syncLimiter)
			res, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(res.RequeueAfter).To(BeNumerically(">", 0))
			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).Should(BeNil(), "Should not set migrationConfiguration")
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.SyncMigrationSlotAnnotation, livemigration.SyncMigrationSlotWaiting))
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.SyncMigrationSourceNodeAnnotation, "node-src"))
		})

		It("Should release the inbound slot when the sync slot is busy", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")
			kvvmi.Status.MigrationState.SourceNode = "node-src"
			kvvmi.Status.MigrationState.TargetNode = "node-tgt"

			otherKVVMI := newKVVMI()
			otherKVVMI.Name = "other-vm"
			withMigrationState(otherKVVMI, "other-migration-uid")
			otherKVVMI.Status.MigrationState.SourceNode = "node-src"

			fakeClient := setupEnvironment(kvvmi, vm, otherKVVMI, newKVConfig())
			inboundLimiter := livemigration.NewInboundMigrationLimiter(true, 1)
			syncLimiter := livemigration.NewSyncMigrationLimiter(true, 1)
			Expect(syncLimiter.TryAcquire(otherKVVMI, "node-src")).To(BeTrue())

			h := NewDynamicSettingsHandler(fakeClient, inboundLimiter, syncLimiter)
			res, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(res.RequeueAfter).To(BeNumerically(">", 0))
			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).Should(BeNil(), "Should not set migrationConfiguration")
			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.SyncMigrationSlotAnnotation, livemigration.SyncMigrationSlotWaiting))

			// The inbound slot taken during the failed acquisition must be handed back,
			// so an unrelated migration can still take it.
			newcomer := newKVVMI()
			newcomer.Name = "newcomer"
			withMigrationState(newcomer, "newcomer-migration-uid")
			Expect(inboundLimiter.TryAcquire(newcomer, "node-tgt")).To(BeTrue())
		})
	})

	newActiveMigration := func(phase virtv1.VirtualMachineInstanceMigrationPhase) *virtv1.VirtualMachineInstanceMigration {
		return &virtv1.VirtualMachineInstanceMigration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: virtv1.SchemeGroupVersion.String(),
				Kind:       virtv1.VirtualMachineInstanceMigrationGroupVersionKind.Kind,
			},
			ObjectMeta: metav1.ObjectMeta{Name: "mig-1", Namespace: vmNamespace},
			Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: vmName},
			Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: phase},
		}
	}

	When("VMI holds a slot but no active migration backs it", func() {
		It("Should release the leaked slot", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")
			livemigration.MarkInboundMigrationSlotAcquired(kvvmi, "node-a")

			fakeClient := setupEnvironment(kvvmi, vm, newKVConfig())
			inboundLimiter := livemigration.NewInboundMigrationLimiter(true, 1)
			Expect(inboundLimiter.TryAcquire(kvvmi, "node-a")).To(BeTrue())

			h := NewDynamicSettingsHandler(fakeClient, inboundLimiter, livemigration.NewSyncMigrationLimiter(false, 1))
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Annotations).NotTo(HaveKey(livemigration.InboundMigrationSlotAnnotation))

			// The freed slot must be available to an unrelated migration.
			newcomer := newKVVMI()
			newcomer.Name = "newcomer"
			withMigrationState(newcomer, "newcomer-uid")
			Expect(inboundLimiter.TryAcquire(newcomer, "node-a")).To(BeTrue())
		})
	})

	When("VMI holds a slot but its MigrationState is gone", func() {
		It("Should release the leaked slot", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")
			livemigration.MarkInboundMigrationSlotAcquired(kvvmi, "node-a")

			inboundLimiter := livemigration.NewInboundMigrationLimiter(true, 1)
			Expect(inboundLimiter.TryAcquire(kvvmi, "node-a")).To(BeTrue())
			kvvmi.Status.MigrationState = nil

			fakeClient := setupEnvironment(kvvmi, vm, newKVConfig())
			h := NewDynamicSettingsHandler(fakeClient, inboundLimiter, livemigration.NewSyncMigrationLimiter(false, 1))
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Annotations).NotTo(HaveKey(livemigration.InboundMigrationSlotAnnotation))

			newcomer := newKVVMI()
			newcomer.Name = "newcomer"
			withMigrationState(newcomer, "newcomer-uid")
			Expect(inboundLimiter.TryAcquire(newcomer, "node-a")).To(BeTrue())
		})
	})

	When("VMI holds a slot while an active migration backs it", func() {
		It("Should keep the slot", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")
			livemigration.MarkInboundMigrationSlotAcquired(kvvmi, "node-a")

			fakeClient := setupEnvironment(kvvmi, vm, newActiveMigration(virtv1.MigrationRunning), newKVConfig())
			inboundLimiter := livemigration.NewInboundMigrationLimiter(true, 1)
			Expect(inboundLimiter.TryAcquire(kvvmi, "node-a")).To(BeTrue())

			h := NewDynamicSettingsHandler(fakeClient, inboundLimiter, livemigration.NewSyncMigrationLimiter(false, 1))
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Annotations).To(HaveKeyWithValue(livemigration.InboundMigrationSlotAnnotation, livemigration.InboundMigrationSlotAcquired))
		})
	})

	When("Observe KVVMI with completed migration", func() {
		It("Should not set migrationConfiguration", func() {
			vm := newVM()
			kvvmi := newKVVMI()

			kvvmi.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{
				Completed: true,
			}

			fakeClient := setupEnvironment(kvvmi, vm, newKVConfig())
			h := NewDynamicSettingsHandler(fakeClient, livemigration.NewInboundMigrationLimiter(true, 1), livemigration.NewSyncMigrationLimiter(false, 1))
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).Should(BeNil(), "Should not set migrationConfiguration")
		})
	})

	DescribeTable("When migration started with VMOP and force flag",
		func(policy v1alpha2.LiveMigrationPolicy, force *bool) {
			vm := newVM()
			vm.Spec.LiveMigrationPolicy = policy

			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")

			vmop := newVMOPEvict(force)

			fakeClient := setupEnvironment(kvvmi, vm, vmop, newKVConfig())
			h := NewDynamicSettingsHandler(fakeClient, livemigration.NewInboundMigrationLimiter(true, 1), livemigration.NewSyncMigrationLimiter(false, 1))
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
			Expect(kvvmi.Status.MigrationState.MigrationConfiguration.AllowAutoConverge).Should(Equal(force))
		},
		Entry("Should enable autoConverge for PreferSafe policy and force=true", v1alpha2.PreferSafeMigrationPolicy, ptr.To(true)),
		Entry("Should disable autoConverge for PreferForced policy and force=false", v1alpha2.PreferForcedMigrationPolicy, ptr.To(false)),
	)
})
