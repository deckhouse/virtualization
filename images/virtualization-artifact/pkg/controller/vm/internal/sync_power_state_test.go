/*
Copyright 2024 Flant JSC

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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Test power actions with VMs", func() {
	var (
		scheme                   *runtime.Scheme
		ctx                      context.Context
		handler                  *SyncPowerStateHandler
		recorderMock             *eventrecord.EventRecorderLoggerMock
		fakeClient               client.Client
		vmState                  state.VirtualMachineState
		vm                       *virtv2.VirtualMachine
		kvvm                     *virtv1.VirtualMachine
		kvvmi                    *virtv1.VirtualMachineInstance
		vmPod                    *corev1.Pod
		namespacedVirtualMachine types.NamespacedName
	)

	AfterEach(func() {
		vm = nil
		kvvm = nil
		kvvmi = nil
		vmPod = nil
		fakeClient = nil
		recorderMock = nil
		vmState = nil
		handler = nil
		scheme = nil
	})

	setupTestEnvironment := func() {
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(vm, kvvm, kvvmi, vmPod).
			WithStatusSubresource(vm, kvvm, kvvmi).
			Build()

		vmResource := reconciler.NewResource(
			types.NamespacedName{Namespace: namespacedVirtualMachine.Namespace, Name: namespacedVirtualMachine.Name},
			fakeClient,
			vmFactoryByVm(vm),
			vmStatusGetter,
		)

		err := vmResource.Fetch(ctx)
		if err != nil {
			return
		}

		vmState = state.New(fakeClient, vmResource)
		handler = NewSyncPowerStateHandler(fakeClient, recorderMock)
	}

	BeforeEach(func() {
		scheme = setupScheme()
		ctx = logger.ToContext(context.TODO(), slog.Default())
		namespacedVirtualMachine = types.NamespacedName{
			Namespace: "vm",
			Name:      "ns",
		}
		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventfFunc: func(client.Object, string, string, string, ...interface{}) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
				return recorderMock
			},
		}

		vm, kvvm, kvvmi, vmPod = createObjectsForPowerstateTest(namespacedVirtualMachine)
	})

	It("should handle start", func() {
		setupKVVMAnnotations(kvvm, annotations.AnnVmStartRequested)
		setupTestEnvironment()

		err := handler.start(ctx, vmState, kvvm, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Status.StateChangeRequests[0].Action).To(Equal(virtv1.StateChangeRequestAction("Start")))
		Expect(kvvm.Annotations[annotations.AnnVmStartRequested]).To(Equal(""))
	})

	It("should handle restart", func() {
		setupKVVMAnnotations(kvvm, annotations.AnnVmRestartRequested)

		setupTestEnvironment()

		err := handler.restart(ctx, vmState, kvvm, kvvmi, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Status.StateChangeRequests[0].Action).To(Equal(virtv1.StateChangeRequestAction("Stop")))
		Expect(kvvm.Status.StateChangeRequests[1].Action).To(Equal(virtv1.StateChangeRequestAction("Start")))
		Expect(kvvm.Annotations[annotations.AnnVmRestartRequested]).To(Equal(""))
	})

	It("should add start annotation", func() {
		kvvm = &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedVirtualMachine.Name,
				Namespace: namespacedVirtualMachine.Namespace,
				Annotations: map[string]string{
					"initFoo": "initBar",
				},
			},
			Spec: virtv1.VirtualMachineSpec{},
		}

		setupTestEnvironment()
		err := handler.restart(ctx, vmState, kvvm, kvvmi, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Annotations[annotations.AnnVmStartRequested]).To(Equal("true"))
	})
})

var _ = Describe("Test action getters for different run policy", func() {
	var (
		scheme                   *runtime.Scheme
		ctx                      context.Context
		handler                  *SyncPowerStateHandler
		recorderMock             *eventrecord.EventRecorderLoggerMock
		fakeClient               client.Client
		vmState                  state.VirtualMachineState
		vm                       *virtv2.VirtualMachine
		kvvm                     *virtv1.VirtualMachine
		kvvmi                    *virtv1.VirtualMachineInstance
		vmPod                    *corev1.Pod
		namespacedVirtualMachine types.NamespacedName
	)

	BeforeEach(func() {
		scheme = setupScheme()
		ctx = logger.ToContext(context.TODO(), slog.Default())
		namespacedVirtualMachine = types.NamespacedName{
			Namespace: "vm",
			Name:      "ns",
		}

		vm, kvvm, kvvmi, vmPod = createObjectsForPowerstateTest(namespacedVirtualMachine)
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(vm, kvvm, kvvmi, vmPod).
			WithStatusSubresource(vm, kvvm, kvvmi).
			Build()

		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventfFunc: func(client.Object, string, string, string, ...interface{}) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger {
				return recorderMock
			},
		}

		vmResource := reconciler.NewResource(
			types.NamespacedName{Namespace: namespacedVirtualMachine.Namespace, Name: namespacedVirtualMachine.Name},
			fakeClient,
			vmFactoryByVm(vm),
			vmStatusGetter,
		)

		err := vmResource.Fetch(ctx)
		if err != nil {
			return
		}

		vmState = state.New(fakeClient, vmResource)
		handler = NewSyncPowerStateHandler(fakeClient, recorderMock)
	})

	AfterEach(func() {
		vm = nil
		kvvm = nil
		kvvmi = nil
		vmPod = nil
		fakeClient = nil
		recorderMock = nil
		vmState = nil
		handler = nil
		scheme = nil
	})

	Context("handleManualPolicy", func() {
		It("should return start action", func() {
			setupKVVMAnnotations(kvvm, annotations.AnnVmStartRequested)

			action := handler.handleManualPolicy(
				ctx, vmState, kvvm, nil, true, powerstate.ShutdownInfo{},
			)

			Expect(action).To(Equal(Start))
		})

		It("should return stop action", func() {
			kvvmi.Status.Phase = virtv1.Succeeded

			action := handler.handleManualPolicy(
				ctx, vmState, kvvm, kvvmi, true, powerstate.ShutdownInfo{PodCompleted: true},
			)

			Expect(action).To(Equal(Stop))
		})

		It("should return restart action", func() {
			setupKVVMAnnotations(kvvm, annotations.AnnVmRestartRequested)
			kvvmi.Status.Phase = virtv1.Running

			action := handler.handleManualPolicy(
				ctx, vmState, kvvm, kvvmi, true, powerstate.ShutdownInfo{},
			)

			Expect(action).To(Equal(Restart))
		})

		It("should return nothing action when conditions are not met", func() {
			kvvmi.Status.Phase = virtv1.Running
			action := handler.handleManualPolicy(
				ctx, vmState, kvvm, kvvmi, false, powerstate.ShutdownInfo{},
			)
			Expect(action).To(Equal(Nothing))
		})
	})

	Context("handleAlwaysOnPolicy", func() {
		It("should return start action when kvvmi is nil and configuration not applied", func() {
			kvvm.Annotations["initFoo"] = "initBar"
			err := fakeClient.Update(context.TODO(), kvvm)
			Expect(err).NotTo(HaveOccurred())

			action, err := handler.handleAlwaysOnPolicy(
				ctx, vmState, kvvm, nil, false, powerstate.ShutdownInfo{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Nothing))
			Expect(kvvm.Annotations[annotations.AnnVmStartRequested]).To(Equal("true"))
		})

		It("should return start action when kvvmi is nil and configuration applied", func() {
			action, err := handler.handleAlwaysOnPolicy(
				ctx, vmState, kvvm, nil, true, powerstate.ShutdownInfo{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Start))
		})

		It("should return nothing action when kvvmi is being deleted", func() {
			kvvmi.DeletionTimestamp = &metav1.Time{}
			action, err := handler.handleAlwaysOnPolicy(
				ctx, vmState, kvvm, kvvmi, true, powerstate.ShutdownInfo{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Nothing))
		})

		It("should return restart action when restart requested", func() {
			setupKVVMAnnotations(kvvm, annotations.AnnVmRestartRequested)
			kvvmi.Status.Phase = virtv1.Running
			action, err := handler.handleAlwaysOnPolicy(
				ctx, vmState, kvvm, kvvmi, true, powerstate.ShutdownInfo{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Restart))
		})

		It("should return restart action when kvvmi on succeeded or failed phase with pod completed", func() {
			kvvmi.Status.Phase = virtv1.Succeeded
			shutdownInfo := powerstate.ShutdownInfo{PodCompleted: true, Reason: powerstate.GuestResetReason}
			action, err := handler.handleAlwaysOnPolicy(
				ctx, vmState, kvvm, kvvmi, true, shutdownInfo,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Restart))
		})

		It("should return nothing action when no conditions are met", func() {
			kvvmi.Status.Phase = virtv1.Running
			action, err := handler.handleAlwaysOnPolicy(
				ctx, vmState, kvvm, kvvmi, true, powerstate.ShutdownInfo{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Nothing))
		})
	})

	Context("handleAlwaysOnUnlessStoppedManuallyPolicy", func() {
		It("should return nothing action when kvvmi is nil", func() {
			action, err := handler.handleAlwaysOnUnlessStoppedManuallyPolicy(
				ctx, vmState, kvvm, nil, true, powerstate.ShutdownInfo{},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Nothing))
		})

		It("should return nothing when kvvmi is being deleted", func() {
			kvvmi.DeletionTimestamp = &metav1.Time{}
			action, err := handler.handleAlwaysOnUnlessStoppedManuallyPolicy(
				ctx, vmState, kvvm, kvvmi, true, powerstate.ShutdownInfo{},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Nothing))
		})

		It("should return restart action when restart requested", func() {
			setupKVVMAnnotations(kvvm, annotations.AnnVmRestartRequested)
			kvvmi.Status.Phase = virtv1.Running
			action, err := handler.handleAlwaysOnUnlessStoppedManuallyPolicy(
				ctx, vmState, kvvm, kvvmi, false, powerstate.ShutdownInfo{},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Restart))
		})

		It("should return restart action on succeeded phase with pod completed and guest reset reason", func() {
			kvvmi.Status.Phase = virtv1.Succeeded
			shutdownInfo := powerstate.ShutdownInfo{PodCompleted: true, Reason: powerstate.GuestResetReason}
			action, err := handler.handleAlwaysOnUnlessStoppedManuallyPolicy(
				ctx, vmState, kvvm, kvvmi, true, shutdownInfo,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Restart))
		})

		It("should return stop action on succeeded phase with pod completed and no guest reset reason", func() {
			kvvmi.Status.Phase = virtv1.Succeeded
			shutdownInfo := powerstate.ShutdownInfo{PodCompleted: true}
			action, err := handler.handleAlwaysOnUnlessStoppedManuallyPolicy(
				ctx, vmState, kvvm, kvvmi, true, shutdownInfo,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Stop))
		})

		It("should return restart action for failed phase", func() {
			kvvmi.Status.Phase = virtv1.Failed
			action, err := handler.handleAlwaysOnUnlessStoppedManuallyPolicy(
				ctx, vmState, kvvm, kvvmi, false, powerstate.ShutdownInfo{},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Restart))
		})

		It("should return nothing action when no conditions are met", func() {
			kvvmi.Status.Phase = virtv1.Running
			action, err := handler.handleAlwaysOnUnlessStoppedManuallyPolicy(
				ctx, vmState, kvvm, kvvmi, true, powerstate.ShutdownInfo{},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(Nothing))
		})
	})

	Context("handleAlwaysOffPolicy", func() {
		It("should return stop action when kvvmi exists", func() {
			action := handler.handleAlwaysOffPolicy(
				ctx, vmState, kvvmi,
			)
			Expect(action).To(Equal(Stop))
		})

		It("should return nothing action when kvvmi is nil", func() {
			action := handler.handleAlwaysOffPolicy(
				ctx, vmState, nil,
			)
			Expect(action).To(Equal(Nothing))
		})
	})
})

func setupScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(virtv2.AddToScheme(scheme)).To(Succeed())
	Expect(virtv1.AddToScheme(scheme)).To(Succeed())
	return scheme
}

func createObjectsForPowerstateTest(namespacedVirtualMachine types.NamespacedName) (*virtv2.VirtualMachine, *virtv1.VirtualMachine, *virtv1.VirtualMachineInstance, *corev1.Pod) {
	vm := &virtv2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedVirtualMachine.Name,
			Namespace: namespacedVirtualMachine.Namespace,
		},
		Status: virtv2.VirtualMachineStatus{},
	}
	kvvm := &virtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        namespacedVirtualMachine.Name,
			Namespace:   namespacedVirtualMachine.Namespace,
			Annotations: map[string]string{},
		},
		Spec: virtv1.VirtualMachineSpec{},
	}
	kvvmi := &virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedVirtualMachine.Name,
			Namespace: namespacedVirtualMachine.Namespace,
		},
		Status: virtv1.VirtualMachineInstanceStatus{},
	}
	vmPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: namespacedVirtualMachine.Namespace,
			Labels:    map[string]string{virtv1.VirtualMachineNameLabel: namespacedVirtualMachine.Name},
		},
	}
	return vm, kvvm, kvvmi, vmPod
}

func setupKVVMAnnotations(kvvm *virtv1.VirtualMachine, key string) {
	kvvm.Annotations[key] = "true"
}
