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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("LifecycleHandler", func() {
	const (
		name      = "test"
		namespace = "default"
	)

	var (
		ctx          context.Context
		fakeClient   client.WithWatch
		srv          *reconciler.Resource[*v1alpha2.VirtualMachineOperation, v1alpha2.VirtualMachineOperationStatus]
		recorderMock *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}
		recorderMock.WithLoggingFunc = func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
			return recorderMock
		}
	})

	newVMOPEvictPending := func(opts ...vmopbuilder.Option) *v1alpha2.VirtualMachineOperation {
		options := []vmopbuilder.Option{
			vmopbuilder.WithName(name),
			vmopbuilder.WithNamespace(namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
			vmopbuilder.WithVirtualMachine(name),
		}
		options = append(options, opts...)
		vmop := vmopbuilder.New(options...)
		vmop.Status.Phase = v1alpha2.VMOPPhasePending
		return vmop
	}

	newVM := func(vmPolicy v1alpha2.LiveMigrationPolicy) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Spec.LiveMigrationPolicy = vmPolicy
		vm.Spec.RunPolicy = v1alpha2.AlwaysOnPolicy
		vm.Status.Phase = v1alpha2.MachineRunning
		vm.Status.Conditions = []metav1.Condition{
			{
				Type:   vmcondition.TypeMigratable.String(),
				Status: metav1.ConditionTrue,
			},
		}

		return vm
	}

	newVMOPMigrate := func(opts ...vmopbuilder.Option) *v1alpha2.VirtualMachineOperation {
		options := []vmopbuilder.Option{
			vmopbuilder.WithName(name),
			vmopbuilder.WithNamespace(namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
			vmopbuilder.WithVirtualMachine(name),
		}
		options = append(options, opts...)
		vmop := vmopbuilder.New(options...)
		return vmop
	}

	DescribeTable("Evict operation for migration policy", func(vmop *v1alpha2.VirtualMachineOperation, vmPolicy v1alpha2.LiveMigrationPolicy, expectedPhase v1alpha2.VMOPPhase) {
		vm := newVM(vmPolicy)

		fakeClient, srv = setupEnvironment(vmop, vm)
		migrationService := service.NewMigrationService(fakeClient, featuregates.Default())
		base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)

		h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)
		_, err := h.Handle(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())

		Expect(srv.Changed().Status.Phase).To(Equal(expectedPhase), "should updated status phase, conditions: %+v", srv.Changed().Status.Conditions)
	},
		// AlwaysSafe cases.
		Entry("is ok for AlwaysSafe and force=nil",
			newVMOPEvictPending(),
			v1alpha2.AlwaysSafeMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),
		Entry("is ok for AlwaysSafe and force=false",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(false))),
			v1alpha2.AlwaysSafeMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),
		Entry("should become Failed for AlwaysSafe and force=true",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(true))),
			v1alpha2.AlwaysSafeMigrationPolicy,
			v1alpha2.VMOPPhaseFailed,
		),

		// PreferSafe cases.
		Entry("is ok for PreferSafe and force=nil",
			newVMOPEvictPending(),
			v1alpha2.PreferSafeMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),
		Entry("is ok for PreferSafe and force=false",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(false))),
			v1alpha2.PreferSafeMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),
		Entry("is ok for PreferSafe and force=true",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(true))),
			v1alpha2.PreferSafeMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),

		// AlwaysForced cases.
		Entry("should become Failed for AlwaysForced and force=nil",
			newVMOPEvictPending(),
			v1alpha2.AlwaysForcedMigrationPolicy,
			v1alpha2.VMOPPhaseFailed,
		),
		Entry("should become Failed for AlwaysForced and force=false",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(false))),
			v1alpha2.AlwaysForcedMigrationPolicy,
			v1alpha2.VMOPPhaseFailed,
		),
		Entry("is ok for AlwaysForced and force=true",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(true))),
			v1alpha2.AlwaysForcedMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),

		// PreferForced cases.
		Entry("is ok for PreferForced and force=nil",
			newVMOPEvictPending(),
			v1alpha2.PreferForcedMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),
		Entry("is ok for PreferForced and force=false",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(false))),
			v1alpha2.PreferForcedMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),
		Entry("is ok for PreferForced and force=true",
			newVMOPEvictPending(vmopbuilder.WithForce(ptr.To(true))),
			v1alpha2.PreferForcedMigrationPolicy,
			v1alpha2.VMOPPhasePending,
		),
	)

	DescribeTable("TargetMigration", func(vmPolicy v1alpha2.LiveMigrationPolicy, nodeSelector map[string]string, targetMigrationEnabled bool) {
		vm := newVM(vmPolicy)
		vm.Status.Conditions = []metav1.Condition{
			{
				Type:   string(vmcondition.TypeMigrating),
				Reason: string(vmcondition.ReasonReadyToMigrate),
			},
		}
		vmop := newVMOPMigrate(vmopbuilder.WithVMOPMigrateNodeSelector(nodeSelector))

		fakeClient, err := testutil.NewFakeClientWithObjects(vmop, vm)
		Expect(err).NotTo(HaveOccurred())

		var (
			featureGate featuregate.FeatureGate
			setFromMap  featuregates.SetFromMapFunc
		)
		if targetMigrationEnabled {
			featureGate, setFromMap, _ = featuregates.NewUnlocked()
			_ = setFromMap(map[string]bool{
				string(featuregates.TargetMigration): true,
			})
		} else {
			featureGate = featuregates.Default()
		}

		migrationService := service.NewMigrationService(fakeClient, featureGate)
		base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)

		h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)
		_, err = h.Handle(ctx, vmop)

		if targetMigrationEnabled {
			Expect(err).NotTo(HaveOccurred())

			vmim := &virtv1.VirtualMachineInstanceMigration{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("vmop-%s", vmop.Name)}, vmim)
			Expect(err).NotTo(HaveOccurred())

			for k, v := range nodeSelector {
				Expect(vmim.Spec.AddedNodeSelector).To(HaveKeyWithValue(k, v))
			}
		} else {
			Expect(err).NotTo(HaveOccurred())
			Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
		}
	},
		Entry(
			"VMIM must have an AddedNodeSelector which is equal to the NodeSelector from VMOP.",
			v1alpha2.PreferSafeMigrationPolicy, // vmPolicy
			map[string]string{"key": "value"},  // nodeSelector
			true,                               // targetMigrationEnabled
		),
		Entry(
			"VMOP should fail with a locked feature error.",
			v1alpha2.PreferSafeMigrationPolicy, // vmPolicy
			map[string]string{"key": "value"},  // nodeSelector
			false,                              // targetMigrationEnabled
		),
	)

	Context("Cancel in-progress migration due to snapshot", func() {
		newInProgressVMOP := func() *v1alpha2.VirtualMachineOperation {
			vmop := vmopbuilder.New(
				vmopbuilder.WithName(name),
				vmopbuilder.WithNamespace(namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
				vmopbuilder.WithVirtualMachine(name),
			)
			vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
			conditions.SetCondition(
				conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
					Reason(vmopcondition.ReasonSignalSentSuccess).
					Status(metav1.ConditionTrue),
				&vmop.Status.Conditions)
			return vmop
		}

		It("Should cancel migration when VM has TypeSnapshotting=True", func() {
			vmop := newInProgressVMOP()

			vm := newVM(v1alpha2.PreferSafeMigrationPolicy)
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:   vmcondition.TypeSnapshotting.String(),
				Status: metav1.ConditionTrue,
				Reason: vmcondition.ReasonSnapshottingInProgress.String(),
			})

			migration := newSimpleMigration(fmt.Sprintf("vmop-%s", name), namespace, name)
			migration.OwnerReferences = []metav1.OwnerReference{
				{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineOperationKind, Name: vmop.Name, UID: vmop.UID, Controller: ptr.To(true)},
			}

			fakeClient, _ = setupEnvironment(vmop, vm, migration)
			migrationService := service.NewMigrationService(fakeClient, featuregates.Default())
			base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)

			h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)
			_, err := h.Handle(ctx, vmop)
			Expect(err).NotTo(HaveOccurred())

			Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
			completed, exists := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(completed.Status).To(Equal(metav1.ConditionFalse))
			Expect(completed.Reason).To(Equal(vmopcondition.ReasonOperationFailed.String()))
			Expect(completed.Message).To(ContainSubstring("snapshot"))
		})

		It("Should cancel migration when attached VD has SnapshottingType=True", func() {
			vdName := "vd-snapshotting"
			vmop := newInProgressVMOP()

			vm := newVM(v1alpha2.PreferSafeMigrationPolicy)
			vm.Status.BlockDeviceRefs = []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: vdName},
			}

			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: vdName, Namespace: namespace},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{Type: vdcondition.SnapshottingType.String(), Status: metav1.ConditionTrue},
					},
				},
			}

			migration := newSimpleMigration(fmt.Sprintf("vmop-%s", name), namespace, name)
			migration.OwnerReferences = []metav1.OwnerReference{
				{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineOperationKind, Name: vmop.Name, UID: vmop.UID, Controller: ptr.To(true)},
			}

			fakeClient, _ = setupEnvironment(vmop, vm, vd, migration)
			migrationService := service.NewMigrationService(fakeClient, featuregates.Default())
			base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)

			h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)
			_, err := h.Handle(ctx, vmop)
			Expect(err).NotTo(HaveOccurred())

			Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
			completed, exists := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(completed.Status).To(Equal(metav1.ConditionFalse))
			Expect(completed.Message).To(ContainSubstring(vdName))
		})

		It("Should not cancel migration when no snapshot is in progress", func() {
			vmop := newInProgressVMOP()

			vm := newVM(v1alpha2.PreferSafeMigrationPolicy)

			migration := newSimpleMigration(fmt.Sprintf("vmop-%s", name), namespace, name)
			migration.OwnerReferences = []metav1.OwnerReference{
				{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineOperationKind, Name: vmop.Name, UID: vmop.UID, Controller: ptr.To(true)},
			}
			migration.Status.Phase = virtv1.MigrationRunning

			fakeClient, _ = setupEnvironment(vmop, vm, migration)
			migrationService := service.NewMigrationService(fakeClient, featuregates.Default())
			base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)

			h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)
			_, err := h.Handle(ctx, vmop)
			Expect(err).NotTo(HaveOccurred())

			Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		})
	})
})
