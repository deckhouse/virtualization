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
	"fmt"
	"log/slog"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
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
				EnableParavirtualization: ptr.To(true),
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

	It("should not hotplug a disk the block device handler marked in use by another VM in this reconcile", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice, Name: "shared-vd",
		})
		kvvm := newKVVM(nil)
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		vd := newVD("shared-vd", "pvc-shared-vd")
		_, _, vmState := setupEnvironment(vm, kvvm, kvvmi, vd)
		vmState.VirtualMachine().Changed().Status.Conditions[0].Status = metav1.ConditionFalse

		result, err := handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
	})

	It("should not hotplug a disk mounted by another VM when run after the block device handler in one reconcile", func() {
		inUse := func(generation int64, attached ...v1alpha2.AttachedVirtualMachine) *v1alpha2.VirtualDisk {
			return &v1alpha2.VirtualDisk{
				Status: v1alpha2.VirtualDiskStatus{
					Phase:                     v1alpha2.DiskReady,
					Target:                    v1alpha2.DiskTarget{PersistentVolumeClaim: "pvc"},
					AttachedToVirtualMachines: attached,
					Conditions: []metav1.Condition{
						{Type: vdcondition.ReadyType.String(), Status: metav1.ConditionTrue, ObservedGeneration: generation},
						{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue, Reason: vdcondition.AttachedToVirtualMachine.String(), ObservedGeneration: generation},
					},
				},
			}
		}
		root := inUse(0, v1alpha2.AttachedVirtualMachine{Name: vmName, Mounted: true})
		root.ObjectMeta = metav1.ObjectMeta{Name: "root-vd", Namespace: vmNamespace}
		shared := inUse(0, v1alpha2.AttachedVirtualMachine{Name: "other-vm", Mounted: true}, v1alpha2.AttachedVirtualMachine{Name: vmName})
		shared.ObjectMeta = metav1.ObjectMeta{Name: "shared-vd", Namespace: vmNamespace}

		vm := newVM(v1alpha2.MachineRunning,
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "root-vd"},
			v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "shared-vd"},
		)
		vm.Status.BlockDeviceRefs = []v1alpha2.BlockDeviceStatusRef{
			{Kind: v1alpha2.DiskDevice, Name: "root-vd", Attached: true},
			{Kind: v1alpha2.DiskDevice, Name: "shared-vd"},
		}
		kvvm := newKVVM([]virtv1.Volume{{
			Name: "vd-root-vd",
			VolumeSource: virtv1.VolumeSource{PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc"},
			}},
		}})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		fakeClient, _, vmState := setupEnvironment(vm, kvvm, kvvmi, root, shared)

		blockDevices := NewBlockDeviceHandler(fakeClient, &BlockDeviceServiceMock{
			CountBlockDevicesAttachedToVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (int, error) { return 2, nil },
		})
		_, err := blockDevices.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		bdReady, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
		Expect(bdReady.Status).To(Equal(metav1.ConditionFalse))

		_, err = handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
	})

	Context("WaitForFirstConsumer disk added to a running VM", func() {
		// newDisk returns a disk this VM mounts; ready=false keeps it in WaitForFirstConsumer.
		newDisk := func(name, pvc string, ready bool, attached ...v1alpha2.AttachedVirtualMachine) *v1alpha2.VirtualDisk {
			phase, readyStatus := v1alpha2.DiskReady, metav1.ConditionTrue
			if !ready {
				phase, readyStatus = v1alpha2.DiskWaitForFirstConsumer, metav1.ConditionFalse
			}
			if len(attached) == 0 {
				attached = []v1alpha2.AttachedVirtualMachine{{Name: vmName, Mounted: true}}
			}
			return &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vmNamespace},
				Status: v1alpha2.VirtualDiskStatus{
					Phase:                     phase,
					Target:                    v1alpha2.DiskTarget{PersistentVolumeClaim: pvc},
					AttachedToVirtualMachines: attached,
					Conditions: []metav1.Condition{
						{Type: vdcondition.ReadyType.String(), Status: readyStatus},
						{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue, Reason: vdcondition.AttachedToVirtualMachine.String()},
					},
				},
			}
		}

		// reconcile runs the block device handler and then the hotplug handler, as one reconcile does.
		reconcileWith := func(extra []v1alpha2.BlockDeviceSpecRef, objs ...client.Object) metav1.Condition {
			GinkgoHelper()
			refs := append([]v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "root-vd"},
				{Kind: v1alpha2.DiskDevice, Name: "wffc-vd"},
			}, extra...)
			vm := newVM(v1alpha2.MachineRunning, refs...)
			kvvm := newKVVM([]virtv1.Volume{{
				Name: "vd-root-vd",
				VolumeSource: virtv1.VolumeSource{PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-root"},
				}},
			}})
			objs = append(objs, kvvm, newEmptyKVVMI(vmName, vmNamespace),
				newDisk("root-vd", "pvc-root", true), newDisk("wffc-vd", "pvc-wffc", false))
			fakeClient, _, vmState := setupEnvironment(vm, objs...)

			blockDevices := NewBlockDeviceHandler(fakeClient, &BlockDeviceServiceMock{
				CountBlockDevicesAttachedToVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (int, error) { return len(refs), nil },
			})
			_, err := blockDevices.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			_, err = handler.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			bdReady, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			return bdReady
		}

		It("should hotplug the disk while the VM waits for it to bind", func() {
			bdReady := reconcileWith(nil)
			Expect(bdReady.Reason).To(Equal(vmcondition.ReasonWaitingForWaitForFirstConsumerBlockDevicesToBeReady.String()))
			Expect(mockSvc.HotPlugDiskCalls()).To(HaveLen(1))
			Expect(mockSvc.HotPlugDiskCalls()[0].Ad.PVCName).To(Equal("pvc-wffc"))
		})

		It("should not hotplug it next to a disk another VM mounts", func() {
			shared := newDisk("shared-vd", "pvc-shared", true,
				v1alpha2.AttachedVirtualMachine{Name: "other-vm", Mounted: true}, v1alpha2.AttachedVirtualMachine{Name: vmName})
			bdReady := reconcileWith([]v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "shared-vd"}}, shared)
			Expect(bdReady.Reason).To(Equal(vmcondition.ReasonBlockDevicesNotReady.String()))
			Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
		})

		It("should not hotplug it next to a disk without storage yet", func() {
			bdReady := reconcileWith([]v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "new-vd"}}, newVD("new-vd", ""))
			Expect(bdReady.Reason).To(Equal(vmcondition.ReasonBlockDevicesNotReady.String()))
			Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
		})
	})

	// Every reason of BlockDevicesReady must be sorted here: a new reason that silently
	// blocks the hotplug leaves a disk added to a running VM unplugged forever.
	DescribeTable("hotplug by the BlockDevicesReady reason",
		func(status metav1.ConditionStatus, reason vmcondition.BlockDevicesReadyReason, hotplug bool) {
			vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: vdName})
			vm.Status.Conditions[0].Status = status
			vm.Status.Conditions[0].Reason = reason.String()

			_, err := runHandle(vm, newKVVM(nil), newEmptyKVVMI(vmName, vmNamespace), newVD(vdName, vdPVCName))
			Expect(err).NotTo(HaveOccurred())
			if hotplug {
				Expect(mockSvc.HotPlugDiskCalls()).To(HaveLen(1))
			} else {
				Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
			}
		},
		Entry(nil, metav1.ConditionTrue, vmcondition.ReasonBlockDevicesReady, true),
		Entry(nil, metav1.ConditionFalse, vmcondition.ReasonWaitingForWaitForFirstConsumerBlockDevicesToBeReady, true),
		Entry(nil, metav1.ConditionFalse, vmcondition.ReasonBlockDevicesNotReady, false),
		Entry(nil, metav1.ConditionFalse, vmcondition.ReasonBlockDeviceLimitExceeded, false),
	)

	It("should still unplug a disk removed from spec when the block device handler marked the VM not ready in this reconcile", func() {
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
		_, _, vmState := setupEnvironment(vm, kvvm, kvvmi)
		vmState.VirtualMachine().Changed().Status.Conditions[0].Status = metav1.ConditionFalse

		result, err := handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.UnplugDiskCalls()).To(HaveLen(1))
	})

	It("should unplug a disk mounted by another VM that keeps the VM not ready after it left the spec", func() {
		ready := []metav1.Condition{{Type: vdcondition.ReadyType.String(), Status: metav1.ConditionTrue}}
		inUse := metav1.Condition{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue, Reason: vdcondition.AttachedToVirtualMachine.String()}
		own := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "own-vd", Namespace: vmNamespace},
			Status: v1alpha2.VirtualDiskStatus{
				Phase:      v1alpha2.DiskReady,
				Target:     v1alpha2.DiskTarget{PersistentVolumeClaim: "pvc-own"},
				Conditions: ready,
			},
		}
		shared := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-vd", Namespace: vmNamespace},
			Status: v1alpha2.VirtualDiskStatus{
				Phase:                     v1alpha2.DiskReady,
				Target:                    v1alpha2.DiskTarget{PersistentVolumeClaim: "pvc-shared"},
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{{Name: "other-vm", Mounted: true}, {Name: vmName}},
				Conditions:                append(slices.Clone(ready), inUse),
			},
		}
		// The spec already points at the own disk, but the shared one is still hotplugged.
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "own-vd"})
		vm.Status.Conditions[0].Status = metav1.ConditionFalse
		vm.Status.BlockDeviceRefs = []v1alpha2.BlockDeviceStatusRef{{Kind: v1alpha2.DiskDevice, Name: "shared-vd", Attached: true, Hotplugged: true}}
		kvvm := newKVVM([]virtv1.Volume{{
			Name: "vd-shared-vd",
			VolumeSource: virtv1.VolumeSource{PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-shared"},
				Hotpluggable:                      true,
			}},
		}})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		fakeClient, _, vmState := setupEnvironment(vm, kvvm, kvvmi, own, shared)

		blockDevices := NewBlockDeviceHandler(fakeClient, &BlockDeviceServiceMock{
			CountBlockDevicesAttachedToVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (int, error) { return 1, nil },
		})
		_, err := blockDevices.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		bdReady, _ := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
		Expect(bdReady.Status).To(Equal(metav1.ConditionFalse))

		_, err = handler.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(mockSvc.UnplugDiskCalls()).To(HaveLen(1))
		Expect(mockSvc.UnplugDiskCalls()[0].DiskName).To(Equal("vd-shared-vd"))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty()) // the own disk comes once the VM is ready
	})

	It("should unplug a replaced disk at once, without waiting for its replacement", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "new-vd"})
		vm.Status.Conditions[0].Status = metav1.ConditionFalse
		kvvm := newKVVM([]virtv1.Volume{{
			Name: "vd-old-vd",
			VolumeSource: virtv1.VolumeSource{PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc-old"},
				Hotpluggable:                      true,
			}},
		}})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)

		result, err := runHandle(vm, kvvm, kvvmi, newVD("new-vd", "pvc-new"))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.UnplugDiskCalls()).To(HaveLen(1))
		Expect(mockSvc.UnplugDiskCalls()[0].DiskName).To(Equal("vd-old-vd"))
		Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty()) // hotplug still waits for the block devices
	})

	It("should not unplug a static disk removed from spec", func() {
		vm := newVM(v1alpha2.MachineRunning)
		kvvm := newKVVM([]virtv1.Volume{{
			Name: "vd-" + vdName,
			VolumeSource: virtv1.VolumeSource{PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: vdPVCName},
			}},
		}})
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)

		result, err := runHandle(vm, kvvm, kvvmi)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.UnplugDiskCalls()).To(BeEmpty())
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

	It("should skip without an error a disk given to another VM right before the hotplug", func() {
		vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: vdName})
		mockSvc.HotPlugDiskFunc = func(_ context.Context, _ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) error {
			return fmt.Errorf("%w: virtual disk %q", service.ErrDiskNotGivenToVM, vdName)
		}

		result, err := runHandle(vm, newKVVM(nil), newEmptyKVVMI(vmName, vmNamespace), newVD(vdName, vdPVCName))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(mockSvc.HotPlugDiskCalls()).To(HaveLen(1))
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

	Context("EnableParavirtualization=false", func() {
		It("should not hotplug a disk", func() {
			vm := newVM(v1alpha2.MachineRunning, v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice, Name: vdName,
			})
			vm.Spec.EnableParavirtualization = ptr.To(false)
			kvvm := newKVVM(nil)
			kvvmi := newEmptyKVVMI(vmName, vmNamespace)
			vd := newVD(vdName, vdPVCName)

			result, err := runHandle(vm, kvvm, kvvmi, vd)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(mockSvc.HotPlugDiskCalls()).To(BeEmpty())
			Expect(mockSvc.UnplugDiskCalls()).To(BeEmpty())
		})

		It("should not unplug a hotpluggable disk removed from spec", func() {
			vm := newVM(v1alpha2.MachineRunning)
			vm.Spec.EnableParavirtualization = ptr.To(false)
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
	})
})
