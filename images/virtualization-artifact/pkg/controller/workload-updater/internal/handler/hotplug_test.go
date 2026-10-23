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

package handler

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/hotplugmigration"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/inplaceresize"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("TestHotplugResourcesHandler", func() {
	const (
		name      = "vm-hotplug-resources"
		namespace = "default"
		nodeName  = "test-node"
		podUID    = types.UID("virt-launcher-test-uid")
	)

	var (
		serviceCompleteErr = errors.New("service is complete")
		ctx                = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient         client.Client
		podCreated         = time.Now().Add(-time.Hour)
	)

	AfterEach(func() {
		fakeClient = nil
	})

	type inPlaceResizeState struct {
		inProgress                bool
		conditionMissing          bool
		conditionReason           string
		podResizePendingReason    string
		podResizeInProgressReason string
	}

	// newVMAndKVVMI returns a migratable virtual machine whose KVVMI asks the runtime for 1 vCPU at 50%.
	newVMAndKVVMI := func(hasHotMemoryChange bool, resizeState inPlaceResizeState) (*v1alpha2.VirtualMachine, *virtv1.VirtualMachineInstance) {
		vm := vmbuilder.NewEmpty(name, namespace)
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.UID = types.UID("kvvmi-test-uid")
		kvvmi.Annotations = map[string]string{kvbuilder.CPUResourcesRequestsFractionAnnotation: "50"}
		kvvmi.Spec.Domain.CPU = &virtv1.CPU{Sockets: 1, Cores: 1, Threads: 1}
		kvvmi.Status.NodeName = nodeName
		kvvmi.Status.ActivePods = map[types.UID]string{podUID: nodeName}
		kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
			Type:   virtv1.VirtualMachineInstanceIsMigratable,
			Status: corev1.ConditionTrue,
		})

		if hasHotMemoryChange {
			kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   virtv1.VirtualMachineInstanceMemoryChange,
				Status: corev1.ConditionTrue,
			})
		}

		if resizeState.inProgress {
			kvvmi.Annotations[virtv1.VirtualMachineInstanceInPlaceResizeInProgressAnn] = "true"
			if !resizeState.conditionMissing {
				kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
					Type:   virtv1.VirtualMachineInstancePodResourceResizeInProgress,
					Status: corev1.ConditionTrue,
					Reason: resizeState.conditionReason,
				})
			}
		}

		return vm, kvvmi
	}

	newLauncherPod := func(kvvmi *virtv1.VirtualMachineInstance, resizeState inPlaceResizeState) *corev1.Pod {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "virt-launcher-test",
				Namespace:         kvvmi.Namespace,
				UID:               podUID,
				CreationTimestamp: metav1.NewTime(podCreated),
				Labels: map[string]string{
					virtv1.AppLabel:       "virt-launcher",
					virtv1.CreatedByLabel: string(kvvmi.UID),
				},
			},
			Spec: corev1.PodSpec{
				NodeName: nodeName,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}

		if resizeState.podResizePendingReason != "" {
			pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
				Type:   corev1.PodResizePending,
				Status: corev1.ConditionTrue,
				Reason: resizeState.podResizePendingReason,
			})
		}
		if resizeState.podResizeInProgressReason != "" {
			pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
				Type:   corev1.PodResizeInProgress,
				Status: corev1.ConditionTrue,
				Reason: resizeState.podResizeInProgressReason,
			})
		}

		return pod
	}

	newHotplugVMOP := func(vmName, key string, phase v1alpha2.VMOPPhase, created, finished time.Time) *v1alpha2.VirtualMachineOperation {
		vmop := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "hotplug-resources-" + created.Format("150405"),
				Namespace:         namespace,
				CreationTimestamp: metav1.NewTime(created),
				Annotations: map[string]string{
					annotations.AnnVMOPWorkloadUpdate: "true",
					hotplugmigration.Annotation:       key,
				},
			},
			Spec:   v1alpha2.VirtualMachineOperationSpec{Type: v1alpha2.VMOPTypeEvict, VirtualMachine: vmName},
			Status: v1alpha2.VirtualMachineOperationStatus{Phase: phase},
		}
		if !finished.IsZero() {
			vmop.Status.Conditions = []metav1.Condition{{
				Type:               vmopcondition.TypeCompleted.String(),
				Status:             metav1.ConditionFalse,
				Reason:             vmopcondition.ReasonOperationFailed.String(),
				LastTransitionTime: metav1.NewTime(finished),
			}}
		}
		return vmop
	}

	newMigrationMock := func(shouldMigrate bool) *HotplugMigrationMock {
		return &HotplugMigrationMock{
			MigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationValue string) (bool, error) {
				if shouldMigrate {
					return true, serviceCompleteErr
				}
				return false, nil
			},
		}
	}

	newGate := func() featuregate.FeatureGate {
		gate, setFromMap, err := featuregates.NewUnlocked()
		Expect(err).NotTo(HaveOccurred())
		Expect(setFromMap(map[string]bool{
			string(featuregates.HotplugMemoryWithLiveMigration):       true,
			string(featuregates.HotplugCPUWithLiveMigration):          true,
			string(featuregates.HotplugCPUAndMemoryWithInPlaceResize): true,
		})).To(Succeed())
		return gate
	}

	kvvmiMark := func() string {
		kvvmi := &virtv1.VirtualMachineInstance{}
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, kvvmi)).To(Succeed())
		return kvvmi.Annotations[hotplugmigration.Annotation]
	}

	type testResourcesSettings struct {
		hasHotMemoryChangeCondition bool
		awaitingRestart             bool
		notMigratable               bool
		shouldMigrate               bool
		// waitTimeoutExpired drops the wait for the resize condition to zero, emulating a runtime
		// that has not confirmed the resize within the timeout.
		waitTimeoutExpired     bool
		expectedMigrationCalls int
		expectedRequeueAfter   time.Duration
		expectedErr            error
		resizeState            inPlaceResizeState
	}

	DescribeTable("HotplugResourcesHandler should return serviceCompleteErr if migration executed",
		func(settings testResourcesSettings) {
			vm, kvvmi := newVMAndKVVMI(settings.hasHotMemoryChangeCondition, settings.resizeState)
			if settings.awaitingRestart {
				vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
					Type:   vmcondition.TypeAwaitingRestartToApplyConfiguration.String(),
					Status: metav1.ConditionTrue,
					Reason: vmcondition.ReasonChangesPendingRestart.String(),
				})
			}
			if settings.notMigratable {
				kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
					Type:   virtv1.VirtualMachineInstanceMigrationTargetAvailable,
					Status: corev1.ConditionFalse,
				})
			}
			fakeClient = setupEnvironment(vm, kvvmi, newLauncherPod(kvvmi, settings.resizeState))

			mockMigration := newMigrationMock(settings.shouldMigrate)
			gate := newGate()
			h := NewHotplugHandler(fakeClient, mockMigration, inplaceresize.New(gate, fakeClient), gate, newNoOpRecorder())
			if settings.waitTimeoutExpired {
				h.inplaceResizeTimeout = 0
			}
			result, err := h.Handle(ctx, vm)

			Expect(mockMigration.MigrateCalls()).To(HaveLen(settings.expectedMigrationCalls))
			Expect(result.RequeueAfter).To(Equal(settings.expectedRequeueAfter))

			if settings.expectedErr != nil {
				Expect(err).To(MatchError(settings.expectedErr))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry(
			"Migration should be executed on hotMemoryChange condition",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               true,
				expectedMigrationCalls:      1,
				expectedErr:                 serviceCompleteErr,
			},
		),
		Entry(
			"Migration should be retried when another operation holds it back",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               false,
				expectedMigrationCalls:      1,
				expectedRequeueAfter:        hotplugMigrationRetryInterval,
			},
		),
		Entry(
			"Migration should not be executed without hotMemoryChange condition",
			testResourcesSettings{
				hasHotMemoryChangeCondition: false,
				expectedMigrationCalls:      0,
			},
		),
		Entry(
			"Migration should not be executed when VM awaits restart to apply configuration",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				awaitingRestart:             true,
				expectedMigrationCalls:      0,
			},
		),
		Entry(
			"Migration should not be executed when no node can take the virtual machine",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				notMigratable:               true,
				expectedMigrationCalls:      0,
			},
		),
		Entry(
			"Migration should not be executed while the runtime applies the changes after a completed in-place resize",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				expectedMigrationCalls:      0,
				expectedRequeueAfter:        time.Second,
				resizeState: inPlaceResizeState{
					inProgress:      true,
					conditionReason: virtv1.VirtualMachineInstanceReasonPodResizeCompleted,
				},
			},
		),
		Entry(
			"Migration should be executed when the runtime has not applied the changes in time after a completed in-place resize",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               true,
				waitTimeoutExpired:          true,
				expectedMigrationCalls:      1,
				expectedErr:                 serviceCompleteErr,
				resizeState: inPlaceResizeState{
					inProgress:      true,
					conditionReason: virtv1.VirtualMachineInstanceReasonPodResizeCompleted,
				},
			},
		),
		Entry(
			"Migration should not be executed when in-place resize is completed and nothing is pending",
			testResourcesSettings{
				hasHotMemoryChangeCondition: false,
				waitTimeoutExpired:          true,
				expectedMigrationCalls:      0,
				resizeState: inPlaceResizeState{
					inProgress:      true,
					conditionReason: virtv1.VirtualMachineInstanceReasonPodResizeCompleted,
				},
			},
		),
		Entry(
			"Migration should not be executed when in-place resize is still possible",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				expectedMigrationCalls:      0,
				resizeState: inPlaceResizeState{
					inProgress:      true,
					conditionReason: virtv1.VirtualMachineInstanceReasonPodResizePending,
				},
			},
		),
		Entry(
			"Migration should be executed when in-place resize is deferred",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               true,
				expectedMigrationCalls:      1,
				expectedErr:                 serviceCompleteErr,
				resizeState: inPlaceResizeState{
					inProgress:             true,
					conditionReason:        virtv1.VirtualMachineInstanceReasonPodResizePending,
					podResizePendingReason: string(corev1.PodReasonDeferred),
				},
			},
		),
		Entry(
			"Migration should be executed when in-place resize is infeasible",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               true,
				expectedMigrationCalls:      1,
				expectedErr:                 serviceCompleteErr,
				resizeState: inPlaceResizeState{
					inProgress:             true,
					conditionReason:        virtv1.VirtualMachineInstanceReasonPodResizeInProgress,
					podResizePendingReason: string(corev1.PodReasonInfeasible),
				},
			},
		),
		Entry(
			"Migration should be executed when in-place resize ended with error",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               true,
				expectedMigrationCalls:      1,
				expectedErr:                 serviceCompleteErr,
				resizeState: inPlaceResizeState{
					inProgress:                true,
					conditionReason:           virtv1.VirtualMachineInstanceReasonPodResizeInProgress,
					podResizeInProgressReason: string(corev1.PodReasonError),
				},
			},
		),
		Entry(
			"Migration should not be executed while the in-place resize condition may still appear",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				expectedMigrationCalls:      0,
				expectedRequeueAfter:        time.Second,
				resizeState: inPlaceResizeState{
					inProgress:       true,
					conditionMissing: true,
				},
			},
		),
		Entry(
			"Migration should be executed when the in-place resize condition has not appeared in time",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               true,
				waitTimeoutExpired:          true,
				expectedMigrationCalls:      1,
				expectedErr:                 serviceCompleteErr,
				resizeState: inPlaceResizeState{
					inProgress:       true,
					conditionMissing: true,
				},
			},
		),
	)

	Describe("migration of a pending configuration", func() {
		var (
			vm    *v1alpha2.VirtualMachine
			kvvmi *virtv1.VirtualMachineInstance
			key   string
		)

		BeforeEach(func() {
			vm, kvvmi = newVMAndKVVMI(true, inPlaceResizeState{})
			key = hotplugmigration.Key(kvvmi)
		})

		handle := func(objs ...client.Object) (*HotplugMigrationMock, time.Duration) {
			GinkgoHelper()
			fakeClient = setupEnvironment(vm, append(objs, kvvmi, newLauncherPod(kvvmi, inPlaceResizeState{}))...)
			mockMigration := newMigrationMock(false)
			gate := newGate()
			h := NewHotplugHandler(fakeClient, mockMigration, inplaceresize.New(gate, fakeClient), gate, newNoOpRecorder())
			result, err := h.Handle(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			return mockMigration, result.RequeueAfter
		}

		It("migrates the configuration without marking a launcher pod no migration has produced", func() {
			mockMigration, _ := handle()
			Expect(mockMigration.MigrateCalls()).To(HaveLen(1))
			Expect(mockMigration.MigrateCalls()[0].AnnotationKey).To(Equal(hotplugmigration.Annotation))
			Expect(mockMigration.MigrateCalls()[0].AnnotationValue).To(Equal(key))
			Expect(kvvmiMark()).To(BeEmpty())
		})

		It("waits before the next attempt after a failed migration", func() {
			failed := newHotplugVMOP(name, key, v1alpha2.VMOPPhaseFailed, time.Now().Add(-time.Minute), time.Now())
			mockMigration, requeueAfter := handle(failed)
			Expect(mockMigration.MigrateCalls()).To(BeEmpty())
			Expect(requeueAfter).To(BeNumerically("~", 5*time.Minute, 5*time.Second))
		})

		It("stops after the last attempt has failed", func() {
			var failed []client.Object
			for i := range hotplugmigration.MaxAttempts {
				finished := time.Now().Add(-time.Duration(hotplugmigration.MaxAttempts-i) * time.Minute)
				failed = append(failed, newHotplugVMOP(name, key, v1alpha2.VMOPPhaseFailed, finished.Add(-time.Second), finished))
			}
			mockMigration, requeueAfter := handle(failed...)
			Expect(mockMigration.MigrateCalls()).To(BeEmpty())
			Expect(requeueAfter).To(BeZero())
		})

		It("migrates once more when the launcher pod a migration produced has not applied the configuration", func() {
			produced := newHotplugVMOP(name, key, v1alpha2.VMOPPhaseCompleted, podCreated.Add(-time.Minute), podCreated.Add(time.Minute))
			kvvmi.Annotations[hotplugmigration.Annotation] = string(podUID) + "/" + key
			mockMigration, _ := handle(produced)
			Expect(mockMigration.MigrateCalls()).To(HaveLen(1))
			Expect(kvvmiMark()).To(Equal(string(podUID) + "/" + key))
		})

		It("does not migrate a third time into the launcher pod the repeated migration produced", func() {
			produced := newHotplugVMOP(name, key, v1alpha2.VMOPPhaseCompleted, podCreated.Add(-time.Minute), podCreated.Add(time.Minute))
			kvvmi.Annotations[hotplugmigration.Annotation] = string(podUID) + "/" + key + "/retried"
			mockMigration, requeueAfter := handle(produced)
			Expect(mockMigration.MigrateCalls()).To(BeEmpty())
			Expect(requeueAfter).To(BeZero())
			Expect(kvvmiMark()).To(Equal(string(podUID) + "/" + key + "/retried"))
		})

		It("checks the configuration again once the runtime has had time to apply it after a migration", func() {
			podCreated = time.Now().Add(-20 * time.Second)
			DeferCleanup(func() { podCreated = time.Now().Add(-time.Hour) })
			produced := newHotplugVMOP(name, key, v1alpha2.VMOPPhaseCompleted, podCreated.Add(-time.Minute), podCreated.Add(time.Second))
			mockMigration, requeueAfter := handle(produced)
			Expect(mockMigration.MigrateCalls()).To(BeEmpty())
			Expect(requeueAfter).To(BeNumerically("~", hotplugmigration.SettleTimeout-20*time.Second, 5*time.Second))
		})

		It("waits for a running migration of the virtual machine", func() {
			running := newHotplugVMOP(name, key, v1alpha2.VMOPPhaseInProgress, time.Now(), time.Time{})
			mockMigration, _ := handle(running)
			Expect(mockMigration.MigrateCalls()).To(BeEmpty())
		})
	})

	It("Drops the configuration a migration produced the launcher pod for when another one is applied in place", func() {
		resizeState := inPlaceResizeState{inProgress: true, conditionReason: virtv1.VirtualMachineInstanceReasonPodResizeInProgress}
		vm, kvvmi := newVMAndKVVMI(false, resizeState)
		migrated := kvvmi.DeepCopy()
		migrated.Annotations[kvbuilder.CPUResourcesRequestsFractionAnnotation] = "10"
		kvvmi.Annotations[hotplugmigration.Annotation] = string(podUID) + "/" + hotplugmigration.Key(migrated)
		fakeClient = setupEnvironment(vm, kvvmi, newLauncherPod(kvvmi, resizeState))

		mockMigration := newMigrationMock(false)
		gate := newGate()
		h := NewHotplugHandler(fakeClient, mockMigration, inplaceresize.New(gate, fakeClient), gate, newNoOpRecorder())
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockMigration.MigrateCalls()).To(BeEmpty())
		Expect(kvvmiMark()).To(Equal(string(podUID) + "/"))
	})

	It("Records the launcher pod a migration produced once another configuration is applied in place", func() {
		// 10% migrated into the pod, which carries the creation second of its migration; 50% is
		// now applied in place, before the mark of the previous pod has been replaced.
		resizeState := inPlaceResizeState{inProgress: true, conditionReason: virtv1.VirtualMachineInstanceReasonPodResizeInProgress}
		vm, kvvmi := newVMAndKVVMI(false, resizeState)
		migrated := kvvmi.DeepCopy()
		migrated.Annotations[kvbuilder.CPUResourcesRequestsFractionAnnotation] = "10"
		produced := newHotplugVMOP(name, hotplugmigration.Key(migrated), v1alpha2.VMOPPhaseCompleted, podCreated, podCreated.Add(14*time.Second))
		kvvmi.Annotations[hotplugmigration.Annotation] = "previous-pod-uid/"
		fakeClient = setupEnvironment(vm, kvvmi, newLauncherPod(kvvmi, resizeState), produced)

		mockMigration := newMigrationMock(false)
		gate := newGate()
		h := NewHotplugHandler(fakeClient, mockMigration, inplaceresize.New(gate, fakeClient), gate, newNoOpRecorder())
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockMigration.MigrateCalls()).To(BeEmpty())
		Expect(kvvmiMark()).To(Equal(string(podUID)+"/"), "a return to 10% must be migrated again")
	})

	It("Announces the fallback to live migration once per desired state", func() {
		resizeState := inPlaceResizeState{inProgress: true, conditionMissing: true}
		vm, kvvmi := newVMAndKVVMI(true, resizeState)
		fakeClient = setupEnvironment(vm, kvvmi, newLauncherPod(kvvmi, resizeState))

		events := 0
		var recorder *eventrecord.EventRecorderLoggerMock
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:           func(_ client.Object, _, _, _ string) { events++ },
			EventfFunc:          func(_ client.Object, _, _, _ string, _ ...any) {},
			AnnotatedEventfFunc: func(_ client.Object, _ map[string]string, _, _, _ string, _ ...any) {},
			// Return the same mock so events recorded through WithLogging are counted too.
			WithLoggingFunc: func(_ eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
		}

		gate := newGate()
		h := NewHotplugHandler(fakeClient, newMigrationMock(false), inplaceresize.New(gate, fakeClient), gate, recorder)
		h.inplaceResizeTimeout = 0

		// The fallback is re-evaluated on every reconcile, but the user must not be flooded.
		for range 5 {
			_, err := h.Handle(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
		}

		Expect(events).To(Equal(1))
	})

	It("Restarts the wait when the desired resources change, so the next resize is not migrated at once", func() {
		resizeState := inPlaceResizeState{inProgress: true, conditionMissing: true}
		vm, kvvmi := newVMAndKVVMI(true, resizeState)
		fakeClient = setupEnvironment(vm, kvvmi, newLauncherPod(kvvmi, resizeState))

		mockMigration := newMigrationMock(true)
		gate := newGate()
		h := NewHotplugHandler(fakeClient, mockMigration, inplaceresize.New(gate, fakeClient), gate, newNoOpRecorder())

		// The runtime never confirms this resize, so it falls back to live migration.
		h.inplaceResizeTimeout = 0
		_, err := h.Handle(ctx, vm)
		Expect(err).To(MatchError(serviceCompleteErr))
		Expect(mockMigration.MigrateCalls()).To(HaveLen(1))

		// A new desired state must get the full timeout instead of being migrated immediately.
		h.inplaceResizeTimeout = time.Minute
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvmi), kvvmi)).To(Succeed())
		kvvmi.Spec.Domain.CPU.Sockets = 2
		Expect(fakeClient.Update(ctx, kvvmi)).To(Succeed())
		result, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Second))
		Expect(mockMigration.MigrateCalls()).To(HaveLen(1))
	})

	It("Keeps the wait after a completed in-place resize while the pending condition flickers", func() {
		resizeState := inPlaceResizeState{inProgress: true, conditionReason: virtv1.VirtualMachineInstanceReasonPodResizeCompleted}
		vm, kvvmi := newVMAndKVVMI(true, resizeState)
		fakeClient = setupEnvironment(vm, kvvmi, newLauncherPod(kvvmi, resizeState))

		mockMigration := newMigrationMock(true)
		gate := newGate()
		h := NewHotplugHandler(fakeClient, mockMigration, inplaceresize.New(gate, fakeClient), gate, newNoOpRecorder())
		h.inplaceResizeTimeout = time.Minute

		setMemoryChange := func(status corev1.ConditionStatus) {
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvmi), kvvmi)).To(Succeed())
			for i := range kvvmi.Status.Conditions {
				if kvvmi.Status.Conditions[i].Type == virtv1.VirtualMachineInstanceMemoryChange {
					kvvmi.Status.Conditions[i].Status = status
				}
			}
			Expect(fakeClient.Status().Update(ctx, kvvmi)).To(Succeed())
		}

		result, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Second))

		// The runtime drops the condition on a failed attempt and puts it back right after.
		setMemoryChange(corev1.ConditionFalse)
		_, err = h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		h.mu.Lock()
		wait, found := h.inplaceResizeWaits[client.ObjectKeyFromObject(vm)]
		wait.since = wait.since.Add(-h.inplaceResizeTimeout)
		h.inplaceResizeWaits[client.ObjectKeyFromObject(vm)] = wait
		h.mu.Unlock()
		Expect(found).To(BeTrue(), "the wait must survive the flicker")

		setMemoryChange(corev1.ConditionTrue)
		_, err = h.Handle(ctx, vm)
		Expect(err).To(MatchError(serviceCompleteErr))
		Expect(mockMigration.MigrateCalls()).To(HaveLen(1))
	})

	DescribeTable("Leaves the KVVMI of a stopped virtual machine alone",
		func(stop func(kvvmi *virtv1.VirtualMachineInstance)) {
			resizeState := inPlaceResizeState{inProgress: true, conditionReason: virtv1.VirtualMachineInstanceReasonPodResizeCompleted}
			vm, kvvmi := newVMAndKVVMI(true, resizeState)
			stop(kvvmi)
			fakeClient = setupEnvironment(vm, kvvmi, newLauncherPod(kvvmi, resizeState))

			events := 0
			var recorder *eventrecord.EventRecorderLoggerMock
			recorder = &eventrecord.EventRecorderLoggerMock{
				EventFunc:           func(_ client.Object, _, _, _ string) { events++ },
				EventfFunc:          func(_ client.Object, _, _, _ string, _ ...any) {},
				AnnotatedEventfFunc: func(_ client.Object, _ map[string]string, _, _, _ string, _ ...any) {},
				WithLoggingFunc:     func(_ eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
			}

			mockMigration := newMigrationMock(true)
			gate := newGate()
			h := NewHotplugHandler(fakeClient, mockMigration, inplaceresize.New(gate, fakeClient), gate, recorder)
			h.inplaceResizeTimeout = 0

			result, err := h.Handle(ctx, vm)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(mockMigration.MigrateCalls()).To(BeEmpty())
			Expect(events).To(BeZero())
		},
		Entry("The guest has shut down", func(kvvmi *virtv1.VirtualMachineInstance) {
			kvvmi.Status.Phase = virtv1.Failed
		}),
		Entry("The KVVMI is being deleted", func(kvvmi *virtv1.VirtualMachineInstance) {
			kvvmi.DeletionTimestamp = ptr.To(metav1.Now())
			kvvmi.Finalizers = []string{"test"}
		}),
	)
})

func newNoOpRecorder() *eventrecord.EventRecorderLoggerMock {
	return &eventrecord.EventRecorderLoggerMock{
		EventFunc:           func(_ client.Object, _, _, _ string) {},
		EventfFunc:          func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		AnnotatedEventfFunc: func(_ client.Object, _ map[string]string, _, _, _ string, _ ...interface{}) {},
		WithLoggingFunc:     func(_ eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return newNoOpRecorder() },
	}
}
