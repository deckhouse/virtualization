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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TestMigrationHandler", func() {
	const (
		namespace = "default"
		vmName    = "test-vm"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.Client
	)

	AfterEach(func() {
		fakeClient = nil
	})

	newVD := func(testName string, storageClassChanged, ready bool) *v1alpha2.VirtualDisk {
		return newTestVD(testName, namespace, vmName, storageClassChanged, ready, false)
	}

	newVM := func() *v1alpha2.VirtualMachine {
		return vmbuilder.NewEmpty(vmName, namespace)
	}

	newVMOP := func(name string, phase v1alpha2.VMOPPhase) *v1alpha2.VirtualMachineOperation {
		return newTestVMOP(name, namespace, vmName, phase)
	}

	newEventRecorder := func() *eventrecord.EventRecorderLoggerMock {
		return &eventrecord.EventRecorderLoggerMock{
			EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
			},
			EventFunc: func(object client.Object, eventtype, reason, message string) {
			},
		}
	}

	It("should skip when storage class not changed", func() {
		vd := newVD("vd-no-change", false, true)
		vm := newVM()
		fakeClient = setupEnvironment(vd, vm)

		h := NewMigrationHandler(fakeClient, newEventRecorder())
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOP was created
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(0))
	})

	It("should skip when VD not ready", func() {
		vd := newVD("vd-not-ready", true, false)
		vm := newVM()
		fakeClient = setupEnvironment(vd, vm)

		h := NewMigrationHandler(fakeClient, newEventRecorder())
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOP was created
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(0))
	})

	It("should do nothing when VM not found", func() {
		vd := newVD("vd-vm-not-found", true, true)
		fakeClient = setupEnvironment(vd)

		eventRecorder := newEventRecorder()
		eventRecorder.EventFunc = func(object client.Object, eventtype, reason, message string) {
			Expect(eventtype).To(Equal(corev1.EventTypeWarning))
			Expect(reason).To(Equal(v1alpha2.ReasonVolumeMigrationCannotBeProcessed))
		}

		h := NewMigrationHandler(fakeClient, eventRecorder)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that no VMOP was created
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(0))
	})

	It("should create VMOP when no existing VMOPs", func() {
		vd := newVD("vd-create-vmop", true, true)
		vm := newVM()
		fakeClient = setupEnvironment(vd, vm)

		h := NewMigrationHandler(fakeClient, newEventRecorder())
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsZero()).To(BeTrue())

		// Check that a VMOP was created
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(1))
	})

	It("should skip when migration already in progress", func() {
		vd := newVD("vd-migration-progress", true, true)
		vm := newVM()
		vmop := newVMOP("volume-migration", v1alpha2.VMOPPhaseInProgress)
		fakeClient = setupEnvironment(vd, vm, vmop)

		h := NewMigrationHandler(fakeClient, newEventRecorder())
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		//nolint:staticcheck // check requeue is not used
		Expect(result.Requeue).To(BeFalse())
		Expect(result.RequeueAfter).To(BeZero())

		// Check that no new VMOP was created
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(1))
	})

	It("should apply backoff when previous migration failed", func() {
		vd := newVD("vd-failed-migration", true, true)
		vm := newVM()
		vmop := newVMOP("volume-migration", v1alpha2.VMOPPhaseFailed)
		vmop.CreationTimestamp = metav1.Now()

		fakeClient = setupEnvironment(vd, vm, vmop)

		eventRecorder := newEventRecorder()
		eventRecorder.EventfFunc = func(involved client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
			Expect(eventtype).To(Equal(corev1.EventTypeNormal))
			Expect(reason).To(Equal(v1alpha2.ReasonVolumeMigrationCannotBeProcessed))
			Expect(messageFmt).To(ContainSubstring("VMOP will be created after the backoff"))
		}

		h := NewMigrationHandler(fakeClient, eventRecorder)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		//nolint:staticcheck // check requeue is not used
		Expect(result.Requeue).To(BeFalse())
		Expect(result.RequeueAfter).To(Equal(5 * time.Second))

		// Check that no new VMOP was created
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(1))
	})

	It("should apply exponential backoff for multiple failed migrations", func() {
		vd := newVD("vd-multiple-failed", true, true)
		vm := newVM()
		vmop1 := newVMOP("volume-migration-1", v1alpha2.VMOPPhaseFailed)
		vmop1.CreationTimestamp = metav1.Now()
		vmop2 := newVMOP("volume-migration-2", v1alpha2.VMOPPhaseFailed)
		vmop2.CreationTimestamp = metav1.Now()

		fakeClient = setupEnvironment(vd, vm, vmop1, vmop2)

		eventRecorder := newEventRecorder()
		eventRecorder.EventfFunc = func(involved client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
			Expect(eventtype).To(Equal(corev1.EventTypeNormal))
			Expect(reason).To(Equal(v1alpha2.ReasonVolumeMigrationCannotBeProcessed))
			Expect(messageFmt).To(ContainSubstring("VMOP will be created after the backoff"))
		}

		h := NewMigrationHandler(fakeClient, eventRecorder)
		result, err := h.Handle(ctx, vd)

		Expect(err).NotTo(HaveOccurred())
		//nolint:staticcheck // check requeue is not used
		Expect(result.Requeue).To(BeFalse())
		Expect(result.RequeueAfter).To(Equal(10 * time.Second))

		// Check that no new VMOP was created
		vmopList := &v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(ctx, vmopList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
		Expect(vmopList.Items).To(HaveLen(2))
	})

	Describe("calculateBackoff", func() {
		var (
			handler *MigrationHandler

			firstTime  metav1.Time
			secondTime metav1.Time
		)

		BeforeEach(func() {
			firstTime = metav1.Now()
			secondTime = metav1.NewTime(firstTime.Add(time.Second))
			handler = NewMigrationHandler(fakeClient, newEventRecorder())
		})

		withCreationTime := func(time metav1.Time, vmops ...*v1alpha2.VirtualMachineOperation) {
			for _, vmop := range vmops {
				vmop.CreationTimestamp = time
			}
		}

		It("should return 0 for no failed VMOPs", func() {
			backoff := handler.calculateBackoff([]*v1alpha2.VirtualMachineOperation{}, firstTime)
			Expect(backoff).To(Equal(time.Duration(0)))
		})

		It("should return 0 for successful VMOPs", func() {
			vmops := []*v1alpha2.VirtualMachineOperation{
				newVMOP("volume-migration", v1alpha2.VMOPPhaseCompleted),
			}

			withCreationTime(secondTime, vmops...)
			backoff := handler.calculateBackoff(vmops, firstTime)
			Expect(backoff).To(Equal(time.Duration(0)))
		})

		It("should calculate exponential backoff for failed VMOPs", func() {
			vmops := []*v1alpha2.VirtualMachineOperation{
				newVMOP("volume-migration-0", v1alpha2.VMOPPhaseFailed),
				newVMOP("volume-migration-1", v1alpha2.VMOPPhaseFailed),
				newVMOP("volume-migration-2", v1alpha2.VMOPPhaseFailed),
				newVMOP("volume-migration-3", v1alpha2.VMOPPhaseFailed),
			}
			withCreationTime(secondTime, vmops...)
			backoff := handler.calculateBackoff(vmops, firstTime)
			Expect(backoff).To(Equal(40 * time.Second))
		})

		It("should cap backoff at maximum delay", func() {
			// Create many failed VMOPs to exceed max delay
			vmops := make([]*v1alpha2.VirtualMachineOperation, 20)
			for i := 0; i < 20; i++ {
				vmops[i] = newVMOP(fmt.Sprintf("volume-migration-%d", i), v1alpha2.VMOPPhaseFailed)
			}
			withCreationTime(secondTime, vmops...)
			backoff := handler.calculateBackoff(vmops, firstTime)
			Expect(backoff).To(Equal(5 * time.Minute)) // max delay
		})
	})
})
