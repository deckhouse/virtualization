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

package internal

import (
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("HotplugHandler", func() {
	const (
		vmName      = "test-vm"
		vmNamespace = "default"
		vdName      = "test-vd"
		vdPVCName   = "pvc-test-vd"
	)

	var (
		ctx     context.Context
		mockSvc *HotplugServiceMock
		handler *HotplugHandler
	)

	BeforeEach(func() {
		ctx = logger.ToContext(context.Background(), slog.Default())
		mockSvc = &HotplugServiceMock{
			HotPlugDiskFunc: func(_ context.Context, _ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) error {
				return nil
			},
			UnplugDiskFunc: func(_ context.Context, _ *virtv1.VirtualMachine, _ string) error {
				return nil
			},
		}
		handler = NewHotplugHandler(mockSvc)
	})

	newVM := func(phase v1alpha2.MachinePhase, bdRefs ...v1alpha2.BlockDeviceSpecRef) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: vmNamespace},
			Spec: v1alpha2.VirtualMachineSpec{
				EnableParavirtualization: true,
				BlockDeviceRefs:          bdRefs,
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: phase,
				Conditions: []metav1.Condition{
					{
						Type:   vmcondition.TypeBlockDevicesReady.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
	}

	newKVVM := func(volumes []virtv1.Volume, volumeRequests ...virtv1.VirtualMachineVolumeRequest) *virtv1.VirtualMachine {
		kvvm := newEmptyKVVM(vmName, vmNamespace)
		kvvm.Spec.Template = &virtv1.VirtualMachineInstanceTemplateSpec{}
		kvvm.Spec.Template.Spec.Volumes = volumes
		kvvm.Status.VolumeRequests = volumeRequests
		return kvvm
	}

	newVD := func(name, pvcName string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vmNamespace},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: pvcName},
			},
		}
	}

	runHandle := func(vm *v1alpha2.VirtualMachine, objs ...client.Object) (reconcile.Result, error) {
		_, _, vmState := setupEnvironment(vm, objs...)
		return handler.Handle(ctx, vmState)
	}

	It("should skip when VM is empty", func() {
		vm := newVM(v1alpha2.MachineRunning)
		vm.Name = ""
		vm.Namespace = ""
		_, _, vmState := setupEnvironment(&v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: vmNamespace},
		})
		result, err := handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
		Expect(mockSvc.UnplugDiskCalls()).To(BeEmpty())
	})

	It("should skip when KVVMI does not exist", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice, Name: vdName,
		})
		kvvm := newKVVM(nil)
		result, err := runHandle(vm, kvvm)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
	})

	It("should skip when VM is migrating", func() {
		vm := newVM(v1alpha2.MachineMigrating, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice, Name: vdName,
		})
		kvvm := newKVVM(nil)
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		result, err := runHandle(vm, kvvm, kvvmi)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
	})

	It("should hotplug a disk that is in spec but not on KVVM", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice, Name: vdName,
		})
		kvvm := newKVVM(nil)
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		vd := newVD(vdName, vdPVCName)

		result, err := runHandle(vm, kvvm, kvvmi, vd)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(HaveLen(1))
		Expect(mockSvc.HotPlugDiskCalls()[0].Ad.PVCName).To(Equal(vdPVCName))
		Expect(mockSvc.UnplugDiskCalls()).To(BeEmpty())
	})

	It("should not hotplug a disk already on KVVM", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice, Name: vdName,
		})
		kvvm := newKVVM([]virtv1.Volume{
			{
				Name: "vd-" + vdName,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: vdPVCName},
						Hotpluggable:                      true,
					},
				},
			},
		})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)

		result, err := runHandle(vm, kvvm, kvvmi)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
		Expect(mockSvc.UnplugDiskCalls()).To(BeEmpty())
	})

	It("should not hotplug a disk with a pending AddVolume request", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice, Name: vdName,
		})
		kvvm := newKVVM(nil, virtv1.VirtualMachineVolumeRequest{
			AddVolumeOptions: &virtv1.AddVolumeOptions{Name: "vd-" + vdName},
		})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		vd := newVD(vdName, vdPVCName)

		result, err := runHandle(vm, kvvm, kvvmi, vd)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
	})

	It("should unplug a hotpluggable disk removed from spec", func() {
		vm := newVM(v1alpha2.MachineRunning)
		kvvm := newKVVM([]virtv1.Volume{
			{
				Name: "vd-" + vdName,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: vdPVCName},
						Hotpluggable:                      true,
					},
				},
			},
		})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)

		result, err := runHandle(vm, kvvm, kvvmi)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.UnplugDiskCalls()).To(HaveLen(1))
		Expect(mockSvc.UnplugDiskCalls()[0].DiskName).To(Equal("vd-" + vdName))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
	})

	It("should not unplug a VMBDA-managed disk", func() {
		vm := newVM(v1alpha2.MachineRunning)
		kvvm := newKVVM([]virtv1.Volume{
			{
				Name: "vd-" + vdName,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: vdPVCName},
						Hotpluggable:                      true,
					},
				},
			},
		})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: "vmbda-test", Namespace: vmNamespace},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: vmName,
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: vdName,
				},
			},
		}

		result, err := runHandle(vm, kvvm, kvvmi, vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.UnplugDiskCalls()).To(BeEmpty())
	})

	It("should not unplug a disk with a pending RemoveVolume request", func() {
		vm := newVM(v1alpha2.MachineRunning)
		kvvm := newKVVM([]virtv1.Volume{
			{
				Name: "vd-" + vdName,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: vdPVCName},
						Hotpluggable:                      true,
					},
				},
			},
		}, virtv1.VirtualMachineVolumeRequest{
			RemoveVolumeOptions: &virtv1.RemoveVolumeOptions{Name: "vd-" + vdName},
		})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)

		result, err := runHandle(vm, kvvm, kvvmi)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.UnplugDiskCalls()).To(BeEmpty())
	})

	It("should skip hotplug when VD has no PVC yet", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice, Name: vdName,
		})
		kvvm := newKVVM(nil)
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		vd := newVD(vdName, "")

		result, err := runHandle(vm, kvvm, kvvmi, vd)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
	})
})
