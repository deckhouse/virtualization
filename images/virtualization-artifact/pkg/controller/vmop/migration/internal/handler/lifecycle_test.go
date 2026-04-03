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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	migrationprogress "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/progress"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration/internal/service"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type progressStrategyStub struct {
	value     int32
	forgotten []types.UID
}

func (s *progressStrategyStub) SyncProgress(_ migrationprogress.Record) int32 {
	return s.value
}

func (s *progressStrategyStub) Forget(uid types.UID) {
	s.forgotten = append(s.forgotten, uid)
}

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

	Describe("migration progress integration", func() {
		It("should return generic failed reason for nil migration", func() {
			h := LifecycleHandler{}

			Expect(h.getFailedReason(nil)).To(Equal(vmopcondition.ReasonFailed))
		})

		DescribeTable("should detect failed reason", func(mig *virtv1.VirtualMachineInstanceMigration, expected vmopcondition.ReasonCompleted) {
			h := LifecycleHandler{}
			Expect(h.getFailedReason(mig)).To(Equal(expected))
		},
			Entry("aborted by request",
				&virtv1.VirtualMachineInstanceMigration{Status: virtv1.VirtualMachineInstanceMigrationStatus{MigrationState: &virtv1.VirtualMachineInstanceMigrationState{AbortRequested: true}}},
				vmopcondition.ReasonAborted,
			),
			Entry("aborted with succeeded status",
				&virtv1.VirtualMachineInstanceMigration{Status: virtv1.VirtualMachineInstanceMigrationStatus{MigrationState: &virtv1.VirtualMachineInstanceMigrationState{AbortStatus: virtv1.MigrationAbortSucceeded}}},
				vmopcondition.ReasonAborted,
			),
			Entry("not converging from failure reason",
				&virtv1.VirtualMachineInstanceMigration{Status: virtv1.VirtualMachineInstanceMigrationStatus{MigrationState: &virtv1.VirtualMachineInstanceMigrationState{FailureReason: "no progress during convergence"}}},
				vmopcondition.ReasonNotConverging,
			),
			Entry("target unschedulable from condition",
				&virtv1.VirtualMachineInstanceMigration{Status: virtv1.VirtualMachineInstanceMigrationStatus{Conditions: []virtv1.VirtualMachineInstanceMigrationCondition{{Type: virtv1.VirtualMachineInstanceMigrationFailed, Reason: "Unschedulable", Message: "pod is unschedulable"}}}},
				vmopcondition.ReasonTargetUnschedulable,
			),
			Entry("target disk error from condition",
				&virtv1.VirtualMachineInstanceMigration{Status: virtv1.VirtualMachineInstanceMigrationStatus{Conditions: []virtv1.VirtualMachineInstanceMigrationCondition{{Type: virtv1.VirtualMachineInstanceMigrationFailed, Reason: "VolumeAttach", Message: "csi volume attach failed"}}}},
				vmopcondition.ReasonTargetDiskError,
			),
			Entry("generic failed reason",
				&virtv1.VirtualMachineInstanceMigration{},
				vmopcondition.ReasonFailed,
			),
		)

		DescribeTable("should build in-progress reason and message", func(
			phase virtv1.VirtualMachineInstanceMigrationPhase,
			state *virtv1.VirtualMachineInstanceMigrationState,
			pod *corev1.Pod,
			expectedReason vmopcondition.ReasonCompleted,
		) {
			mig := newSimpleMigration("vmop-test", name)
			mig.UID = "migration-uid"
			mig.Status.Phase = phase
			mig.Status.MigrationState = state

			objects := []client.Object{mig}
			if pod != nil {
				objects = append(objects, pod)
			}
			fakeClient, err := testutil.NewFakeClientWithObjects(objects...)
			Expect(err).NotTo(HaveOccurred())

			h := LifecycleHandler{client: fakeClient}
			reason, _, err := h.getInProgressReasonAndMessage(ctx, mig)
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal(expectedReason))
		},
			Entry("phase unset means target scheduling",
				virtv1.MigrationPhaseUnset,
				nil,
				nil,
				vmopcondition.ReasonTargetScheduling,
			),
			Entry("scheduled means target preparing",
				virtv1.MigrationScheduled,
				nil,
				nil,
				vmopcondition.ReasonTargetPreparing,
			),
			Entry("running means syncing",
				virtv1.MigrationRunning,
				nil,
				nil,
				vmopcondition.ReasonSyncing,
			),
			Entry("unschedulable pod has priority",
				virtv1.MigrationScheduling,
				nil,
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "target-pod",
						Labels: map[string]string{
							virtv1.AppLabel:          "virt-launcher",
							virtv1.MigrationJobLabel: "migration-uid",
						},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionFalse,
							Reason: corev1.PodReasonUnschedulable,
						}},
					},
				},
				vmopcondition.ReasonTargetUnschedulable,
			),
			Entry("target resumed after domain ready timestamp",
				virtv1.MigrationRunning,
				&virtv1.VirtualMachineInstanceMigrationState{TargetNodeDomainReadyTimestamp: &metav1.Time{Time: time.Now()}},
				nil,
				vmopcondition.ReasonTargetResumed,
			),
			Entry("source suspended after completed flag",
				virtv1.MigrationRunning,
				&virtv1.VirtualMachineInstanceMigrationState{Completed: true},
				nil,
				vmopcondition.ReasonSourceSuspended,
			),
		)

		DescribeTable("should map progress by reason", func(reason vmopcondition.ReasonCompleted, initial *int32, expected int32) {
			h := LifecycleHandler{progressStrategy: &progressStrategyStub{value: 55}}
			vmop := &v1alpha2.VirtualMachineOperation{Status: v1alpha2.VirtualMachineOperationStatus{Progress: initial}}
			mig := &virtv1.VirtualMachineInstanceMigration{}

			Expect(h.calculateMigrationProgress(vmop, mig, reason)).To(Equal(expected))
		},
			Entry("disks preparing", vmopcondition.ReasonDisksPreparing, nil, int32(1)),
			Entry("target scheduling", vmopcondition.ReasonTargetScheduling, nil, int32(2)),
			Entry("target unschedulable", vmopcondition.ReasonTargetUnschedulable, nil, int32(2)),
			Entry("target preparing", vmopcondition.ReasonTargetPreparing, nil, int32(3)),
			Entry("target disk error", vmopcondition.ReasonTargetDiskError, nil, int32(3)),
			Entry("syncing delegates to strategy", vmopcondition.ReasonSyncing, nil, int32(55)),
			Entry("source suspended", vmopcondition.ReasonSourceSuspended, nil, int32(91)),
			Entry("target resumed", vmopcondition.ReasonTargetResumed, nil, int32(92)),
			Entry("migration completed", vmopcondition.ReasonMigrationCompleted, nil, int32(100)),
			Entry("unknown keeps existing progress", vmopcondition.ReasonFailed, ptr.To[int32](44), int32(44)),
		)

		It("should set syncing progress inside [10,90] for running migration", func() {
			vm := newVM(v1alpha2.PreferSafeMigrationPolicy)
			vmop := newVMOPMigrate()
			vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
			vmop.Status.Progress = ptr.To[int32](10)

			mig := newSimpleMigration(fmt.Sprintf("vmop-%s", vmop.Name), name)
			mig.Status.Phase = virtv1.MigrationRunning
			mig.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{
				StartTimestamp: &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
			}

			fakeClient, srv = setupEnvironment(vmop, vm, mig)
			migrationService := service.NewMigrationService(fakeClient, featuregates.Default())
			base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)
			h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)

			_, err := h.Handle(ctx, srv.Changed())
			Expect(err).NotTo(HaveOccurred())
			Expect(srv.Changed().Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
			Expect(srv.Changed().Status.Progress).NotTo(BeNil())
			Expect(*srv.Changed().Status.Progress).To(BeNumerically(">=", migrationprogress.SyncRangeMin))
			Expect(*srv.Changed().Status.Progress).To(BeNumerically("<=", migrationprogress.SyncRangeMax))

			completed, found := conditions.GetCondition(vmopcondition.TypeCompleted, srv.Changed().Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(completed.Reason).To(Equal(vmopcondition.ReasonSyncing.String()))
		})

		It("should set pending phase and progress to 2 for scheduling migration", func() {
			vm := newVM(v1alpha2.PreferSafeMigrationPolicy)
			vmop := newVMOPMigrate()
			vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress

			mig := newSimpleMigration(fmt.Sprintf("vmop-%s", vmop.Name), name)
			mig.Status.Phase = virtv1.MigrationScheduling

			fakeClient, srv = setupEnvironment(vmop, vm, mig)
			migrationService := service.NewMigrationService(fakeClient, featuregates.Default())
			base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)
			h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)

			_, err := h.Handle(ctx, srv.Changed())
			Expect(err).NotTo(HaveOccurred())
			Expect(srv.Changed().Status.Phase).To(Equal(v1alpha2.VMOPPhasePending))
			Expect(srv.Changed().Status.Progress).NotTo(BeNil())
			Expect(*srv.Changed().Status.Progress).To(Equal(int32(2)))
		})

		It("should set aborted reason and preserve progress for failed migration", func() {
			vm := newVM(v1alpha2.PreferSafeMigrationPolicy)
			vmop := newVMOPMigrate()
			vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress
			vmop.Status.Progress = ptr.To[int32](55)

			mig := newSimpleMigration(fmt.Sprintf("vmop-%s", vmop.Name), name)
			mig.Status.Phase = virtv1.MigrationFailed
			mig.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{AbortRequested: true}

			fakeClient, srv = setupEnvironment(vmop, vm, mig)
			migrationService := service.NewMigrationService(fakeClient, featuregates.Default())
			base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)
			h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)

			_, err := h.Handle(ctx, srv.Changed())
			Expect(err).NotTo(HaveOccurred())
			Expect(srv.Changed().Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
			Expect(srv.Changed().Status.Progress).NotTo(BeNil())
			Expect(*srv.Changed().Status.Progress).To(Equal(int32(55)))

			completed, found := conditions.GetCondition(vmopcondition.TypeCompleted, srv.Changed().Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(completed.Reason).To(Equal(vmopcondition.ReasonAborted.String()))
		})

		It("should set progress to 100 for succeeded migration", func() {
			vm := newVM(v1alpha2.PreferSafeMigrationPolicy)
			vmop := newVMOPMigrate()
			vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress

			mig := newSimpleMigration(fmt.Sprintf("vmop-%s", vmop.Name), name)
			mig.Status.Phase = virtv1.MigrationSucceeded

			fakeClient, srv = setupEnvironment(vmop, vm, mig)
			migrationService := service.NewMigrationService(fakeClient, featuregates.Default())
			base := genericservice.NewBaseVMOPService(fakeClient, recorderMock)
			h := NewLifecycleHandler(fakeClient, migrationService, base, recorderMock)

			_, err := h.Handle(ctx, srv.Changed())
			Expect(err).NotTo(HaveOccurred())
			Expect(srv.Changed().Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
			Expect(srv.Changed().Status.Progress).NotTo(BeNil())
			Expect(*srv.Changed().Status.Progress).To(Equal(int32(100)))
		})
	})
})
