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
	"context"
	"fmt"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("Test BlockDeviceReady condition", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	okBlockDeviceServiceMock := &BlockDeviceServiceMock{
		CountBlockDevicesAttachedToVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (int, error) {
			return 1, nil
		},
	}

	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		v1alpha2.AddToScheme,
		virtv1.AddToScheme,
		corev1.AddToScheme,
	} {
		err := f(scheme)
		Expect(err).NotTo(HaveOccurred(), "failed to add scheme: %s", err)
	}

	namespacedName := types.NamespacedName{
		Namespace: "ns",
		Name:      "vm",
	}

	getVMWithOneVD := func(phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
			},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: "vd1",
					},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: phase,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: "vd1",
					},
				},
			},
		}
	}

	getNotReadyVD := func(name string, status metav1.ConditionStatus, reason string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespacedName.Namespace,
			},
			Status: v1alpha2.VirtualDiskStatus{
				Conditions: []metav1.Condition{{
					Type:   vdcondition.InUseType.String(),
					Status: status,
					Reason: reason,
				}},
			},
		}
	}

	nameVD1 := "vd1"
	nameVD2 := "vd2"

	DescribeTable("One not ready disk", func(vd *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine, status metav1.ConditionStatus, msg string) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd).Build()

		vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
		err := vmResource.Fetch(ctx)
		Expect(err).NotTo(HaveOccurred())

		vmState := state.New(fakeClient, vmResource)
		handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
		_, err = handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
		Expect(bdCond.Message).To(Equal(msg))
		Expect(bdCond.Status).To(Equal(status))
	},
		Entry(
			"vd AttachedToVirtualMachine & Pending VM",
			getNotReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
		Entry(
			"vd AttachedToVirtualMachine & Running VM",
			getNotReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
		Entry(
			"vd AttachedToVirtualMachine & Stopped VM",
			getNotReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
		// --
		Entry(
			"vd UsedForImageCreation & Pending VM",
			getNotReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithOneVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
		Entry(
			"vd UsedForImageCreation & Running VM",
			getNotReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithOneVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
		Entry(
			"vd UsedForImageCreation & Stopped VM",
			getNotReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithOneVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
		// --
		Entry(
			"vd NotInUse & Pending VM",
			getNotReadyVD(nameVD1, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
		Entry(
			"vd NotInUse & Running VM",
			getNotReadyVD(nameVD1, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
		Entry(
			"vd NotInUse & Stopped VM",
			getNotReadyVD(nameVD1, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready.",
		),
	)

	getWFFCVD := func(status metav1.ConditionStatus, reason string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vd1",
				Namespace: namespacedName.Namespace,
			},
			Status: v1alpha2.VirtualDiskStatus{
				Phase: v1alpha2.DiskWaitForFirstConsumer,
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "testPvc",
				},
				Conditions: []metav1.Condition{{
					Type:   vdcondition.InUseType.String(),
					Status: status,
					Reason: reason,
				}},
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
					{
						Name:    namespacedName.Name,
						Mounted: true,
					},
				},
			},
		}
	}

	DescribeTable("One wffc disk", func(vd *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine, status metav1.ConditionStatus, msg string) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd).Build()

		vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
		err := vmResource.Fetch(ctx)
		Expect(err).NotTo(HaveOccurred())

		vmState := state.New(fakeClient, vmResource)
		handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
		_, err = handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
		Expect(bdCond.Message).To(Equal(msg))
		Expect(bdCond.Status).To(Equal(status))
	},
		Entry(
			"vd AttachedToVirtualMachine & Pending VM",
			getWFFCVD(metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready; Virtual disk vd1 is waiting for the underlying PVC to be bound.",
		),
		Entry(
			"vd AttachedToVirtualMachine & Running VM",
			getWFFCVD(metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready; Virtual disk vd1 is waiting for the underlying PVC to be bound.",
		),
		Entry(
			"vd AttachedToVirtualMachine & Stopped VM",
			getWFFCVD(metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready; Virtual disk vd1 is waiting for the virtual machine to be starting.",
		),
		// --
		Entry(
			"vd NotInUse & Pending VM",
			getWFFCVD(metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready to use.",
		),
		Entry(
			"vd NotInUse & Running VM",
			getWFFCVD(metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready to use.",
		),
		Entry(
			"vd NotInUse & Stopped VM",
			getWFFCVD(metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready; Virtual disk vd1 is waiting for the virtual machine to be starting.",
		),
	)

	getReadyVD := func(name string, status metav1.ConditionStatus, reason string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespacedName.Namespace,
			},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "testPvc",
				},
				Phase: v1alpha2.DiskReady,
				Conditions: []metav1.Condition{
					{
						Type:    vdcondition.ReadyType.String(),
						Status:  metav1.ConditionTrue,
						Reason:  vdcondition.Ready.String(),
						Message: "",
					},
					{
						Type:   vdcondition.InUseType.String(),
						Status: status,
						Reason: reason,
					},
				},
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
					{
						Name:    namespacedName.Name,
						Mounted: true,
					},
				},
			},
		}
	}

	DescribeTable("One ready disk", func(vd *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine, status metav1.ConditionStatus, msg string) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd).Build()

		vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
		err := vmResource.Fetch(ctx)
		Expect(err).NotTo(HaveOccurred())

		vmState := state.New(fakeClient, vmResource)
		handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
		_, err = handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
		Expect(bdCond.Message).To(Equal(msg))
		Expect(bdCond.Status).To(Equal(status))
	},
		Entry(
			"vd AttachedToVirtualMachine & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachinePending),
			metav1.ConditionTrue,
			"",
		),
		Entry(
			"vd AttachedToVirtualMachine & Running VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachineRunning),
			metav1.ConditionTrue,
			"",
		),
		Entry(
			"vd AttachedToVirtualMachine & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithOneVD(v1alpha2.MachineStopped),
			metav1.ConditionTrue,
			"",
		),
		// --
		Entry(
			"vd UsedForImageCreation & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithOneVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Virtual disk \"vd1\" is in use for image creation.",
		),
		Entry(
			"vd UsedForImageCreation & Running VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithOneVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Virtual disk \"vd1\" is in use for image creation.",
		),
		Entry(
			"vd UsedForImageCreation & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithOneVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Virtual disk \"vd1\" is in use for image creation.",
		),
		// --
		Entry(
			"vd NotInUse & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready to use.",
		),
		Entry(
			"vd NotInUse & Running VM",
			getReadyVD(nameVD1, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block device \"vd1\" to be ready to use.",
		),
		Entry(
			"vd NotInUse & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithOneVD(v1alpha2.MachineStopped),
			metav1.ConditionTrue,
			"",
		),
	)

	getVMWithTwoVD := func(phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
			},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: "vd1",
					},
					{
						Kind: v1alpha2.DiskDevice,
						Name: "vd2",
					},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: phase,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{
						Kind: v1alpha2.DiskDevice,
						Name: "vd1",
					},
					{
						Kind: v1alpha2.DiskDevice,
						Name: "vd2",
					},
				},
			},
		}
	}

	DescribeTable("two disks: not ready disk & ready disk", func(vd1, vd2 *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine, status metav1.ConditionStatus, msg string) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd1, vd2).Build()

		vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
		err := vmResource.Fetch(ctx)
		Expect(err).NotTo(HaveOccurred())

		vmState := state.New(fakeClient, vmResource)
		handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
		_, err = handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
		Expect(bdCond.Message).To(Equal(msg))
		Expect(bdCond.Status).To(Equal(status))
	},
		Entry(
			"vd2 AttachedToVirtualMachine & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithTwoVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
		Entry(
			"vd2 AttachedToVirtualMachine & Running VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithTwoVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
		Entry(
			"vd2 AttachedToVirtualMachine & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithTwoVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
		// --
		Entry(
			"vd2 UsedForImageCreation & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithTwoVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
		Entry(
			"vd2 UsedForImageCreation & Running VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithTwoVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
		Entry(
			"vd2 UsedForImageCreation & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithTwoVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
		// --
		Entry(
			"vd NotInUse & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithTwoVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
		Entry(
			"vd2 NotInUse & Running VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithTwoVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
		Entry(
			"vd2 NotInUse & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getNotReadyVD(nameVD2, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithTwoVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready: 1/2.",
		),
	)

	DescribeTable("two disks: two ready disks", func(vd1, vd2 *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine, status metav1.ConditionStatus, msg string) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd1, vd2).Build()

		vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
		err := vmResource.Fetch(ctx)
		Expect(err).NotTo(HaveOccurred())

		vmState := state.New(fakeClient, vmResource)
		handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
		_, err = handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())

		bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
		Expect(bdCond.Message).To(Equal(msg))
		Expect(bdCond.Status).To(Equal(status))
	},
		Entry(
			"vd2 AttachedToVirtualMachine & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithTwoVD(v1alpha2.MachinePending),
			metav1.ConditionTrue,
			"",
		),
		Entry(
			"vd2 AttachedToVirtualMachine & Running VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithTwoVD(v1alpha2.MachineRunning),
			metav1.ConditionTrue,
			"",
		),
		Entry(
			"vd2 AttachedToVirtualMachine & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getVMWithTwoVD(v1alpha2.MachineStopped),
			metav1.ConditionTrue,
			"",
		),
		// --
		Entry(
			"vd2 UsedForImageCreation & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithTwoVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready to use: 1/2; Virtual disk \"vd2\" is in use for image creation.",
		),
		Entry(
			"vd2 UsedForImageCreation & Running VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithTwoVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready to use: 1/2; Virtual disk \"vd2\" is in use for image creation.",
		),
		Entry(
			"vd2 UsedForImageCreation & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionTrue, vdcondition.UsedForImageCreation.String()),
			getVMWithTwoVD(v1alpha2.MachineStopped),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready to use: 1/2; Virtual disk \"vd2\" is in use for image creation.",
		),
		// --
		Entry(
			"vd NotInUse & Pending VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithTwoVD(v1alpha2.MachinePending),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready to use: 1/2.",
		),
		Entry(
			"vd2 NotInUse & Running VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithTwoVD(v1alpha2.MachineRunning),
			metav1.ConditionFalse,
			"Waiting for block devices to be ready to use: 1/2.",
		),
		Entry(
			"vd2 NotInUse & Stopped VM",
			getReadyVD(nameVD1, metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String()),
			getReadyVD(nameVD2, metav1.ConditionFalse, vdcondition.NotInUse.String()),
			getVMWithTwoVD(v1alpha2.MachineStopped),
			metav1.ConditionTrue,
			"",
		),
	)

	Context("three not ready disks", func() {
		It("blockDeviceReady condition set Status = False and Message = Waiting for block devices to be ready: 0/3.", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd2",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd3",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd2",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd3",
						},
					},
				},
			}
			vd1 := getNotReadyVD("vd1", metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String())
			vd2 := getNotReadyVD("vd2", metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String())
			vd3 := getNotReadyVD("vd3", metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String())
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd1, vd2, vd3).Build()

			vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
			err := vmResource.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			vmState := state.New(fakeClient, vmResource)
			handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
			_, err = handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(bdCond.Message).To(Equal("Waiting for block devices to be ready: 0/3."))
			Expect(bdCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("five disks: "+
		"- two not ready for use disks, "+
		"- one ready disk, "+
		"- two disk using for create image", func() {
		It("blockDeviceReady condition set Status = False and complex message.", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd2",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd3",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd4",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd5",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd2",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd3",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd4",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd5",
						},
					},
				},
			}
			vd1 := getReadyVD("vd1", metav1.ConditionFalse, vdcondition.NotInUse.String())
			vd2 := getReadyVD("vd2", metav1.ConditionFalse, vdcondition.NotInUse.String())
			vd3 := getReadyVD("vd3", metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String())
			vd4 := getReadyVD("vd4", metav1.ConditionTrue, vdcondition.UsedForImageCreation.String())
			vd5 := getReadyVD("vd5", metav1.ConditionTrue, vdcondition.UsedForImageCreation.String())
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd1, vd2, vd3, vd4, vd5).Build()

			vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
			err := vmResource.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			vmState := state.New(fakeClient, vmResource)
			handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
			_, err = handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(bdCond.Message).To(Equal("Waiting for block devices to be ready to use: 1/5; Virtual disks 2/5 are in use for image creation."))
			Expect(bdCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("five disks: "+
		"- two not ready for use disks, "+
		"- one ready disk, one disk using for create image, "+
		"- one disk attached to another vm", func() {
		It("blockDeviceReady condition set Status = False and complex message.", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd2",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd3",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd4",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd5",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd2",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd3",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd4",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd5",
						},
					},
				},
			}
			vd1 := getReadyVD("vd1", metav1.ConditionFalse, vdcondition.NotInUse.String())
			vd2 := getReadyVD("vd2", metav1.ConditionFalse, vdcondition.NotInUse.String())
			vd3 := getReadyVD("vd3", metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String())
			vd4 := getReadyVD("vd4", metav1.ConditionTrue, vdcondition.UsedForImageCreation.String())
			vd5 := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vd5",
					Namespace: namespacedName.Namespace,
				},
				Status: v1alpha2.VirtualDiskStatus{
					Target: v1alpha2.DiskTarget{
						PersistentVolumeClaim: "testPvc",
					},
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{
						{
							Type:    vdcondition.ReadyType.String(),
							Status:  metav1.ConditionTrue,
							Reason:  vdcondition.Ready.String(),
							Message: "",
						},
						{
							Type:   vdcondition.InUseType.String(),
							Status: metav1.ConditionTrue,
							Reason: vdcondition.AttachedToVirtualMachine.String(),
						},
					},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{
							Name:    "a-vm",
							Mounted: true,
						},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd1, vd2, vd3, vd4, vd5).Build()

			vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
			err := vmResource.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			vmState := state.New(fakeClient, vmResource)
			handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
			_, err = handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(bdCond.Message).To(Equal("Waiting for block devices to be ready to use: 1/5; Virtual disk \"vd4\" is in use for image creation; Virtual disk \"vd5\" is in use by another VM."))
			Expect(bdCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("five disks: "+
		"- one ready disk, "+
		"- two disks using for create image, "+
		"- two disks attached to another vm", func() {
		It("blockDeviceReady condition set Status = False and complex message.", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd2",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd3",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd4",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd5",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd2",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd3",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd4",
						},
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd5",
						},
					},
				},
			}
			vd1 := getReadyVD("vd1", metav1.ConditionTrue, vdcondition.AttachedToVirtualMachine.String())
			vd2 := getReadyVD("vd2", metav1.ConditionTrue, vdcondition.UsedForImageCreation.String())
			vd3 := getReadyVD("vd3", metav1.ConditionTrue, vdcondition.UsedForImageCreation.String())
			vd4 := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vd4",
					Namespace: namespacedName.Namespace,
				},
				Status: v1alpha2.VirtualDiskStatus{
					Target: v1alpha2.DiskTarget{
						PersistentVolumeClaim: "testPvc",
					},
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{
						{
							Type:    vdcondition.ReadyType.String(),
							Status:  metav1.ConditionTrue,
							Reason:  vdcondition.Ready.String(),
							Message: "",
						},
						{
							Type:   vdcondition.InUseType.String(),
							Status: metav1.ConditionTrue,
							Reason: vdcondition.AttachedToVirtualMachine.String(),
						},
					},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{
							Name:    "b-vm",
							Mounted: true,
						},
					},
				},
			}
			vd5 := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vd5",
					Namespace: namespacedName.Namespace,
				},
				Status: v1alpha2.VirtualDiskStatus{
					Target: v1alpha2.DiskTarget{
						PersistentVolumeClaim: "testPvc",
					},
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{
						{
							Type:    vdcondition.ReadyType.String(),
							Status:  metav1.ConditionTrue,
							Reason:  vdcondition.Ready.String(),
							Message: "",
						},
						{
							Type:   vdcondition.InUseType.String(),
							Status: metav1.ConditionTrue,
							Reason: vdcondition.AttachedToVirtualMachine.String(),
						},
					},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{
							Name:    "a-vm",
							Mounted: true,
						},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd1, vd2, vd3, vd4, vd5).Build()

			vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
			err := vmResource.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			vmState := state.New(fakeClient, vmResource)
			handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
			_, err = handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(bdCond.Message).To(Equal("Waiting for block devices to be ready to use: 1/5; Virtual disks 2/5 are in use for image creation; Virtual disks 2/5 are in use by another VM."))
			Expect(bdCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("one disk attached to another vm", func() {
		It("blockDeviceReady condition set Status = False and Message = Virtual disk \"vd1\" is in use by another VM.", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
					},
				},
			}
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vd1",
					Namespace: namespacedName.Namespace,
				},
				Status: v1alpha2.VirtualDiskStatus{
					Target: v1alpha2.DiskTarget{
						PersistentVolumeClaim: "testPvc",
					},
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{
						{
							Type:    vdcondition.ReadyType.String(),
							Status:  metav1.ConditionTrue,
							Reason:  vdcondition.Ready.String(),
							Message: "",
						},
						{
							Type:   vdcondition.InUseType.String(),
							Status: metav1.ConditionTrue,
							Reason: vdcondition.AttachedToVirtualMachine.String(),
						},
					},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{
							Name:    "a-vm",
							Mounted: true,
						},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd).Build()

			vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
			err := vmResource.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			vmState := state.New(fakeClient, vmResource)
			handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
			_, err = handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(bdCond.Message).To(Equal("Virtual disk \"vd1\" is in use by another VM."))
			Expect(bdCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("one not ready disk attached to another vm", func() {
		It("return false and message = Waiting for block device \"vd1\" to be ready", func() {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "vd1",
						},
					},
				},
			}
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vd1",
					Namespace: namespacedName.Namespace,
				},
				Status: v1alpha2.VirtualDiskStatus{
					Target: v1alpha2.DiskTarget{
						PersistentVolumeClaim: "testPvc",
					},
					Phase: v1alpha2.DiskProvisioning,
					Conditions: []metav1.Condition{
						{
							Type:    vdcondition.ReadyType.String(),
							Status:  metav1.ConditionFalse,
							Reason:  vdcondition.Provisioning.String(),
							Message: "",
						},
						{
							Type:   vdcondition.InUseType.String(),
							Status: metav1.ConditionTrue,
							Reason: vdcondition.AttachedToVirtualMachine.String(),
						},
					},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{
							Name:    "a-vm",
							Mounted: true,
						},
					},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, vd).Build()

			vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
			err := vmResource.Fetch(ctx)
			Expect(err).NotTo(HaveOccurred())

			vmState := state.New(fakeClient, vmResource)
			handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
			_, err = handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			bdCond, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(bdCond.Message).To(Equal("Waiting for block device \"vd1\" to be ready; Virtual disk vd1 is waiting for the underlying PVC to be bound."))
			Expect(bdCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(bdCond.Reason).To(Equal(vmcondition.ReasonBlockDevicesNotReady.String()))
		})
	})
})

var _ = Describe("BlockDeviceHandler", func() {
	var h *BlockDeviceHandler
	var vm *v1alpha2.VirtualMachine
	var vi *v1alpha2.VirtualImage
	var cvi *v1alpha2.ClusterVirtualImage
	var vdFoo *v1alpha2.VirtualDisk
	var vdBar *v1alpha2.VirtualDisk

	blockDeviceHandlerMock := &BlockDeviceServiceMock{}
	blockDeviceHandlerMock.CountBlockDevicesAttachedToVMFunc = func(_ context.Context, vm *v1alpha2.VirtualMachine) (int, error) {
		return 1, nil
	}

	getBlockDevicesState := func(vi *v1alpha2.VirtualImage, cvi *v1alpha2.ClusterVirtualImage, vdFoo, vdBar *v1alpha2.VirtualDisk) BlockDevicesState {
		return BlockDevicesState{
			VIByName:  map[string]*v1alpha2.VirtualImage{vi.Name: vi},
			CVIByName: map[string]*v1alpha2.ClusterVirtualImage{cvi.Name: cvi},
			VDByName:  map[string]*v1alpha2.VirtualDisk{vdFoo.Name: vdFoo, vdBar.Name: vdBar},
		}
	}

	BeforeEach(func() {
		h = NewBlockDeviceHandler(nil, blockDeviceHandlerMock)
		vi = &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "vi-01"},
			Status:     v1alpha2.VirtualImageStatus{Phase: v1alpha2.ImageReady},
		}
		cvi = &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "cvi-01"},
			Status:     v1alpha2.ClusterVirtualImageStatus{Phase: v1alpha2.ImageReady},
		}
		vdFoo = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd1-foo"},
			Status: v1alpha2.VirtualDiskStatus{
				Phase:  v1alpha2.DiskReady,
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: "pvc-foo"},
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.ReadyType.String(),
						Reason: vdcondition.Ready.String(),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   vdcondition.InUseType.String(),
						Reason: vdcondition.AttachedToVirtualMachine.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
		vdBar = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd1-bar"},
			Status: v1alpha2.VirtualDiskStatus{
				Phase:  v1alpha2.DiskReady,
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: "pvc-bar"},
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.ReadyType.String(),
						Reason: vdcondition.Ready.String(),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   vdcondition.InUseType.String(),
						Reason: vdcondition.AttachedToVirtualMachine.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
		vm = &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{Name: vi.Name, Kind: v1alpha2.ImageDevice},
					{Name: cvi.Name, Kind: v1alpha2.ClusterImageDevice},
					{Name: vdFoo.Name, Kind: v1alpha2.DiskDevice},
					{Name: vdBar.Name, Kind: v1alpha2.DiskDevice},
				},
			},
		}
	})

	Context("VirtualMachine is nil", func() {
		It("Not ready, cannot start, no warnings", func() {
			ready, canStart, warnings := h.countReadyBlockDevices(nil, BlockDevicesState{}, false)
			Expect(ready).To(Equal(0))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})
	})

	Context("BlockDevices are ready", func() {
		It("Ready, can start, no warnings", func() {
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, false)
			Expect(ready).To(Equal(4))
			Expect(canStart).To(BeTrue())
			Expect(warnings).To(BeNil())
		})
	})

	Context("Image is not ready", func() {
		It("VirtualImage not ready: cannot start, no warnings", func() {
			vi.Status.Phase = v1alpha2.ImagePending
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, false)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})

		It("ClusterVirtualImage not ready: cannot start, no warnings", func() {
			cvi.Status.Phase = v1alpha2.ImagePending
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, false)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})
	})

	Context("VirtualDisk is not ready", func() {
		It("VirtualDisk's target pvc is not yet created", func() {
			vdFoo.Status.Phase = v1alpha2.DiskProvisioning
			vdFoo.Status.Target.PersistentVolumeClaim = ""
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, false)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})

		It("VirtualDisk's target pvc is created", func() {
			vdFoo.Status.Phase = v1alpha2.DiskProvisioning
			vdFoo.Status.Conditions = []metav1.Condition{
				{
					Type:   vdcondition.ReadyType.String(),
					Reason: vdcondition.Provisioning.String(),
					Status: metav1.ConditionFalse,
				},
				{
					Type:   vdcondition.InUseType.String(),
					Reason: vdcondition.AttachedToVirtualMachine.String(),
					Status: metav1.ConditionTrue,
				},
			}
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, false)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeTrue())
			Expect(warnings).ToNot(BeEmpty())
		})
	})
})

var _ = Describe("Capacity check", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	Context("Handle call result based on the number of connected block devices", func() {
		scheme := apiruntime.NewScheme()
		for _, f := range []func(*apiruntime.Scheme) error{
			v1alpha2.AddToScheme,
			virtv1.AddToScheme,
			corev1.AddToScheme,
		} {
			err := f(scheme)
			Expect(err).NotTo(HaveOccurred(), "failed to add scheme: %s", err)
		}

		namespacedName := types.NamespacedName{
			Namespace: "ns",
			Name:      "vm",
		}

		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
			},
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{},
			},
		}

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
			},
			Spec: v1alpha2.VirtualMachineSpec{},
			Status: v1alpha2.VirtualMachineStatus{
				Conditions: []metav1.Condition{
					{
						Status:  metav1.ConditionUnknown,
						Type:    vmcondition.TypeBlockDevicesReady.String(),
						Reason:  conditions.ReasonUnknown.String(),
						Message: "",
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, kvvm).Build()
		vmResource := reconciler.NewResource(namespacedName, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
		_ = vmResource.Fetch(ctx)

		vmState := state.New(fakeClient, vmResource)

		It("Should be ok because fewer than 16 devices are connected", func() {
			okBlockDeviceServiceMock := &BlockDeviceServiceMock{
				CountBlockDevicesAttachedToVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (int, error) {
					return 1, nil
				},
			}

			handler := NewBlockDeviceHandler(fakeClient, okBlockDeviceServiceMock)
			result, err := handler.Handle(ctx, vmState)
			Expect(err).To(BeNil())
			Expect(result).To(Equal(reconcile.Result{}))
			readyCondition, ok := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal(vmcondition.ReasonBlockDevicesReady.String()))
		})
		It("There might be an issue since 16 or more devices are connected.", func() {
			erroredBlockDeviceServiceMock := &BlockDeviceServiceMock{
				CountBlockDevicesAttachedToVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (int, error) {
					return 17, nil
				},
			}

			handler := NewBlockDeviceHandler(fakeClient, erroredBlockDeviceServiceMock)
			result, err := handler.Handle(ctx, vmState)
			Expect(err).To(BeNil())
			Expect(result).To(Equal(reconcile.Result{}))
			readyCondition, ok := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal(vmcondition.ReasonBlockDeviceLimitExceeded.String()))
		})
	})

	Context("When images are hotplugged into a VirtualMachine", func() {
		It("checks that `VirtualMachine.Status.BlockDeviceRefs` contains the hotplugged images", func() {
			blockDeviceServiceMock := &BlockDeviceServiceMock{
				CountBlockDevicesAttachedToVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (int, error) {
					return 2, nil
				},
			}

			scheme := apiruntime.NewScheme()
			for _, f := range []func(*apiruntime.Scheme) error{
				v1alpha2.AddToScheme,
				virtv1.AddToScheme,
			} {
				err := f(scheme)
				Expect(err).NotTo(HaveOccurred(), "failed to add scheme: %s", err)
			}

			namespacedVirtualMachine := types.NamespacedName{
				Namespace: "hotplugged",
				Name:      "vm-with-hotplugged-images",
			}

			namespacedVirtualImage := types.NamespacedName{
				Namespace: "hotplugged",
				Name:      "vi-hotplug",
			}

			namespacedClusterVirtualImage := types.NamespacedName{
				Name: "cvi-hotplug",
			}

			vi := &v1alpha2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedVirtualImage.Name,
					Namespace: namespacedVirtualImage.Namespace,
				},
				Spec: v1alpha2.VirtualImageSpec{},
				Status: v1alpha2.VirtualImageStatus{
					Phase: v1alpha2.ImageReady,
					Size: v1alpha2.ImageStatusSize{
						Unpacked: "200Mi",
					},
				},
			}

			cvi := &v1alpha2.ClusterVirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespacedClusterVirtualImage.Name,
				},
				Spec: v1alpha2.ClusterVirtualImageSpec{},
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase: v1alpha2.ImageReady,
					Size: v1alpha2.ImageStatusSize{
						Unpacked: "200Mi",
					},
				},
			}

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedVirtualMachine.Name,
					Namespace: namespacedVirtualMachine.Namespace,
				},
				Spec: v1alpha2.VirtualMachineSpec{},
				Status: v1alpha2.VirtualMachineStatus{
					Conditions: []metav1.Condition{
						{
							Status:  metav1.ConditionUnknown,
							Type:    vmcondition.TypeBlockDevicesReady.String(),
							Reason:  conditions.ReasonUnknown.String(),
							Message: "",
						},
					},
				},
			}

			cviTarget := "sdb"
			viTarget := "sdc"

			kvvm := &virtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedVirtualMachine.Name,
					Namespace: namespacedVirtualMachine.Namespace,
				},
				Spec: virtv1.VirtualMachineSpec{
					Template: &virtv1.VirtualMachineInstanceTemplateSpec{},
				},
			}

			kvvmi := &virtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedVirtualMachine.Name,
					Namespace: namespacedVirtualMachine.Namespace,
				},
				Status: virtv1.VirtualMachineInstanceStatus{
					VolumeStatus: []virtv1.VolumeStatus{
						{
							Name:   fmt.Sprintf("cvi-%s", namespacedClusterVirtualImage.Name),
							Target: cviTarget,
							Phase:  virtv1.VolumeReady,
						},
						{
							Name:   fmt.Sprintf("vi-%s", namespacedVirtualImage.Name),
							Target: viTarget,
							Phase:  virtv1.VolumeReady,
						},
					},
				},
			}

			vmbdaVi := &v1alpha2.VirtualMachineBlockDeviceAttachment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedVirtualImage.Name,
					Namespace: namespacedVirtualImage.Namespace,
				},
				Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
					VirtualMachineName: namespacedVirtualMachine.Name,
					BlockDeviceRef: v1alpha2.VMBDAObjectRef{
						Kind: v1alpha2.VMBDAObjectRefKindVirtualImage,
						Name: namespacedVirtualImage.Name,
					},
				},
				Status: v1alpha2.VirtualMachineBlockDeviceAttachmentStatus{
					Phase: v1alpha2.BlockDeviceAttachmentPhaseAttached,
				},
			}

			vmbdaCvi := &v1alpha2.VirtualMachineBlockDeviceAttachment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedClusterVirtualImage.Name,
					Namespace: namespacedVirtualMachine.Namespace,
				},
				Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
					VirtualMachineName: namespacedVirtualMachine.Name,
					BlockDeviceRef: v1alpha2.VMBDAObjectRef{
						Kind: v1alpha2.VMBDAObjectRefKindClusterVirtualImage,
						Name: namespacedClusterVirtualImage.Name,
					},
				},
				Status: v1alpha2.VirtualMachineBlockDeviceAttachmentStatus{
					Phase: v1alpha2.BlockDeviceAttachmentPhaseAttached,
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, kvvm, kvvmi, vi, cvi, vmbdaVi, vmbdaCvi).Build()
			vmResource := reconciler.NewResource(namespacedVirtualMachine, fakeClient, vmFactoryByVM(vm), vmStatusGetter)
			_ = vmResource.Fetch(ctx)

			vmState := state.New(fakeClient, vmResource)

			handler := NewBlockDeviceHandler(fakeClient, blockDeviceServiceMock)
			_, err := handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred(), "failed to handle VirtualMachineState: %s", err)
			vm = vmState.VirtualMachine().Changed()
			for _, bd := range vm.Status.BlockDeviceRefs {
				Expect(bd.Attached).To(BeTrue(), "`attached` field should be `true`")
				Expect(bd.Hotplugged).To(BeTrue(), "`hotplugged` field should be `true`")
				switch bd.Kind {
				case v1alpha2.ClusterVirtualImageKind:
					Expect(bd.Name).To(Equal(namespacedClusterVirtualImage.Name), "`Name` should be %q", namespacedClusterVirtualImage.Name)
					Expect(bd.VirtualMachineBlockDeviceAttachmentName).To(Equal(namespacedClusterVirtualImage.Name), "`VirtualMachineBlockDeviceAttachmentName` should be %q", namespacedClusterVirtualImage.Name)
					Expect(bd.Size).To(Equal(cvi.Status.Size.Unpacked), "unpacked size of image should be %s", cvi.Status.Size.Unpacked)
					Expect(bd.Target).To(Equal(cviTarget), "`target` field should be %s", cviTarget)
				case v1alpha2.VirtualImageKind:
					Expect(bd.Name).To(Equal(namespacedVirtualImage.Name), "`Name` should be %q", namespacedVirtualImage.Name)
					Expect(bd.VirtualMachineBlockDeviceAttachmentName).To(Equal(namespacedVirtualImage.Name), "`VirtualMachineBlockDeviceAttachmentName` should be %q", namespacedVirtualImage.Name)
					Expect(bd.Size).To(Equal(vi.Status.Size.Unpacked), "unpacked size of image should be %s", vi.Status.Size.Unpacked)
					Expect(bd.Target).To(Equal(viTarget), "`target` field should be %s", viTarget)
				}
			}
		})
	})
})

func vmFactoryByVM(vm *v1alpha2.VirtualMachine) func() *v1alpha2.VirtualMachine {
	return func() *v1alpha2.VirtualMachine {
		return vm
	}
}

func vmStatusGetter(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus {
	return obj.Status
}
