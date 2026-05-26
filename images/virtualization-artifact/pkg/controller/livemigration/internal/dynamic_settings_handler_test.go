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
			h := NewDynamicSettingsHandler(fakeClient)
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
			Expect(kvvmi.Annotations).NotTo(HaveKey(livemigration.InboundMigrationSlotAnnotation))
		})

		It("Should wait without migrationConfiguration when inbound slot is busy", func() {
			vm := newVM()
			kvvmi := newKVVMI()
			withMigrationState(kvvmi, "migration-uid")

			otherKVVMI := newKVVMI()
			otherKVVMI.Name = "other-vm"
			withMigrationState(otherKVVMI, "other-migration-uid")

			fakeClient := setupEnvironment(kvvmi, vm, otherKVVMI, newKVConfig())
			limiter := livemigration.NewInboundMigrationLimiter(fakeClient)
			acquired, err := limiter.TryAcquire(ctx, otherKVVMI, "node-a", livemigration.ParallelInboundMigrationsPerNodeDefault)
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired).To(BeTrue())

			h := NewDynamicSettingsHandler(fakeClient)
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
			h := NewDynamicSettingsHandler(fakeClient)
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
			Expect(kvvmi.Status.MigrationState.MigrationConfiguration.DisableTLS).ShouldNot(BeNil(), "Should propagate DisableTLS")
			Expect(*kvvmi.Status.MigrationState.MigrationConfiguration.DisableTLS).To(BeTrue())
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
			h := NewDynamicSettingsHandler(fakeClient)
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
			h := NewDynamicSettingsHandler(fakeClient)
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
			Expect(kvvmi.Status.MigrationState.MigrationConfiguration.AllowAutoConverge).Should(Equal(force))
		},
		Entry("Should enable autoConverge for PreferSafe policy and force=true", v1alpha2.PreferSafeMigrationPolicy, ptr.To(true)),
		Entry("Should disable autoConverge for PreferForced policy and force=false", v1alpha2.PreferForcedMigrationPolicy, ptr.To(false)),
	)
})
