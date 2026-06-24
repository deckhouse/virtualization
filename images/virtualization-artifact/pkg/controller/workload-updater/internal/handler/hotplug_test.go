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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/inplaceresize"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("TestHotplugResourcesHandler", func() {
	const (
		name      = "vm-hotplug-resources"
		namespace = "default"
	)

	var (
		serviceCompleteErr = errors.New("service is complete")
		ctx                = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient         client.Client
	)

	AfterEach(func() {
		fakeClient = nil
	})

	type inPlaceResizeState struct {
		inProgress                bool
		conditionReason           string
		podResizePendingReason    string
		podResizeInProgressReason string
	}

	newVMAndKVVMI := func(hasHotMemoryChange bool, resizeState inPlaceResizeState) (*v1alpha2.VirtualMachine, *virtv1.VirtualMachineInstance) {
		vm := vmbuilder.NewEmpty(name, namespace)
		kvvmi := newEmptyKVVMI(name, namespace)

		if hasHotMemoryChange {
			kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   virtv1.VirtualMachineInstanceMemoryChange,
				Status: corev1.ConditionTrue,
			})
		}

		if resizeState.inProgress {
			if kvvmi.Annotations == nil {
				kvvmi.Annotations = make(map[string]string)
			}
			kvvmi.Annotations[virtv1.VirtualMachineInstanceInPlaceResizeInProgressAnn] = "true"
			kvvmi.Status.Conditions = append(kvvmi.Status.Conditions, virtv1.VirtualMachineInstanceCondition{
				Type:   virtv1.VirtualMachineInstancePodResourceResizeInProgress,
				Status: corev1.ConditionTrue,
				Reason: resizeState.conditionReason,
			})
		}

		return vm, kvvmi
	}

	newLauncherPod := func(kvvmi *virtv1.VirtualMachineInstance, resizeState inPlaceResizeState) *corev1.Pod {
		if !resizeState.inProgress || resizeState.conditionReason == virtv1.VirtualMachineInstanceReasonPodResizeCompleted {
			return nil
		}

		const (
			nodeName = "test-node"
			podName  = "virt-launcher-test"
		)

		podUID := types.UID("virt-launcher-test-uid")
		kvvmi.UID = types.UID("kvvmi-test-uid")
		kvvmi.Status.NodeName = nodeName
		kvvmi.Status.ActivePods = map[types.UID]string{
			podUID: nodeName,
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: kvvmi.Namespace,
				UID:       podUID,
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

	newOnceMigrationMock := func(shouldMigrate bool) *OneShotMigrationMock {
		return &OneShotMigrationMock{
			OnceMigrateFunc: func(ctx context.Context, vm *v1alpha2.VirtualMachine, annotationKey, annotationExpectedValue string) (bool, error) {
				if shouldMigrate {
					return true, serviceCompleteErr
				}
				return false, nil
			},
		}
	}

	type testResourcesSettings struct {
		hasHotMemoryChangeCondition bool
		awaitingRestart             bool
		shouldMigrate               bool
		expectedMigrationCalls      int
		expectedErr                 error
		resizeState                 inPlaceResizeState
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
			pod := newLauncherPod(kvvmi, settings.resizeState)
			if pod != nil {
				fakeClient = setupEnvironment(vm, kvvmi, pod)
			} else {
				fakeClient = setupEnvironment(vm, kvvmi)
			}

			mockMigration := newOnceMigrationMock(settings.shouldMigrate)

			gate, setFromMap, err := featuregates.NewUnlocked()
			Expect(err).NotTo(HaveOccurred())

			err = setFromMap(map[string]bool{
				string(featuregates.HotplugMemoryWithLiveMigration):       true,
				string(featuregates.HotplugCPUWithLiveMigration):          true,
				string(featuregates.HotplugCPUAndMemoryWithInPlaceResize): true,
			})
			Expect(err).NotTo(HaveOccurred())

			h := NewHotplugHandler(fakeClient, mockMigration, inplaceresize.New(gate, fakeClient), gate, newNoOpRecorder())
			_, err = h.Handle(ctx, vm)

			Expect(mockMigration.OnceMigrateCalls()).To(HaveLen(settings.expectedMigrationCalls))

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
			"Migration should not return an error when one-shot migration reports no action",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
				shouldMigrate:               false,
				expectedMigrationCalls:      1,
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
			"Migration should not be executed when in-place resize is already completed",
			testResourcesSettings{
				hasHotMemoryChangeCondition: true,
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
