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

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TestDynamicSettingsHandler", func() {
	const (
		vmName      = "vm-migratable"
		vmNamespace = "default"
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	newVM := func() *virtv2.VirtualMachine {
		vm := vmbuilder.NewEmpty(vmName, vmNamespace)
		vm.Spec.LiveMigrationPolicy = virtv2.PreferSafeMigrationPolicy

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

	newVMOPEvict := func(force *bool) *virtv2.VirtualMachineOperation {
		vmop := &virtv2.VirtualMachineOperation{
			TypeMeta: metav1.TypeMeta{
				APIVersion: virtv2.SchemeGroupVersion.String(),
				Kind:       virtv2.VirtualMachineOperationKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-vmop-name",
				Namespace: vmNamespace,
			},
			Spec: virtv2.VirtualMachineOperationSpec{
				Type:           virtv2.VMOPTypeEvict,
				VirtualMachine: vmName,
				Force:          force,
			},
			Status: virtv2.VirtualMachineOperationStatus{
				Phase: virtv2.VMOPPhaseInProgress,
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

			kvvmi.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{}

			fakeClient := setupEnvironment(kvvmi, vm, newKVConfig())
			h := NewDynamicSettingsHandler(fakeClient)
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
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
		func(policy virtv2.LiveMigrationPolicy, force *bool) {
			vm := newVM()
			vm.Spec.LiveMigrationPolicy = policy

			kvvmi := newKVVMI()
			kvvmi.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{}

			vmop := newVMOPEvict(force)

			fakeClient := setupEnvironment(kvvmi, vm, vmop, newKVConfig())
			h := NewDynamicSettingsHandler(fakeClient)
			_, err := h.Handle(ctx, kvvmi)
			Expect(err).NotTo(HaveOccurred())

			Expect(kvvmi.Status.MigrationState.MigrationConfiguration).ShouldNot(BeNil(), "Should set migrationConfiguration")
			Expect(kvvmi.Status.MigrationState.MigrationConfiguration.AllowAutoConverge).Should(Equal(force))
		},
		Entry("Should enable autoConverge for PreferSafe policy and force=true", virtv2.PreferSafeMigrationPolicy, ptr.To(true)),
		Entry("Should disable autoConverge for PreferForced policy and force=false", virtv2.PreferForcedMigrationPolicy, ptr.To(false)),
	)
})
