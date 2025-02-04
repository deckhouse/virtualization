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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

func TestBlockDeviceHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BlockDeviceHandler Suite")
}

var _ = Describe("func areVirtualDisksAllowedToUse", func() {
	var h *BlockDeviceHandler
	var vdFoo *virtv2.VirtualDisk
	var vdBar *virtv2.VirtualDisk

	blockDeviceHandlerMock := &BlockDeviceServiceMock{}
	blockDeviceHandlerMock.CountBlockDevicesAttachedToVmFunc = func(_ context.Context, vm *virtv2.VirtualMachine) (int, error) {
		return 1, nil
	}

	BeforeEach(func() {
		h = NewBlockDeviceHandler(nil, &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}, blockDeviceHandlerMock)
		vdFoo = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-foo"},
			Status: virtv2.VirtualDiskStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.InUseType.String(),
						Reason: vdcondition.AttachedToVirtualMachine.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
		vdBar = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-bar"},
			Status: virtv2.VirtualDiskStatus{
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.InUseType.String(),
						Reason: vdcondition.AttachedToVirtualMachine.String(),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
	})

	Context("VirtualDisk is not allowed", func() {
		It("returns false", func() {
			anyVd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "anyVd"},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{
							Type:   vdcondition.InUseType.String(),
							Reason: conditions.ReasonUnknown.String(),
							Status: metav1.ConditionUnknown,
						},
					},
				},
			}

			vds := map[string]*virtv2.VirtualDisk{
				vdFoo.Name: vdFoo,
				vdBar.Name: vdBar,
				anyVd.Name: anyVd,
			}

			allowed := h.areVirtualDisksAllowedToUse(vds)
			Expect(allowed).To(BeFalse())
		})
	})

	Context("VirtualDisk is used to create image", func() {
		It("returns false", func() {
			anyVd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: "anyVd"},
				Status: virtv2.VirtualDiskStatus{
					Phase:  virtv2.DiskReady,
					Target: virtv2.DiskTarget{PersistentVolumeClaim: "pvc-foo"},
					Conditions: []metav1.Condition{
						{
							Type:   vdcondition.ReadyType.String(),
							Reason: vdcondition.Ready.String(),
							Status: metav1.ConditionTrue,
						},
						{
							Type:   vdcondition.InUseType.String(),
							Reason: vdcondition.UsedForImageCreation.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			vds := map[string]*virtv2.VirtualDisk{
				vdFoo.Name: vdFoo,
				vdBar.Name: vdBar,
				anyVd.Name: anyVd,
			}

			allowed := h.areVirtualDisksAllowedToUse(vds)
			Expect(allowed).To(BeFalse())
		})
	})

	Context("VirtualDisk is allowed", func() {
		It("returns true", func() {
			vds := map[string]*virtv2.VirtualDisk{
				vdFoo.Name: vdFoo,
				vdBar.Name: vdBar,
			}

			allowed := h.areVirtualDisksAllowedToUse(vds)
			Expect(allowed).To(BeTrue())
		})
	})
})

var _ = Describe("BlockDeviceHandler", func() {
	var h *BlockDeviceHandler
	var logger *slog.Logger
	var vm *virtv2.VirtualMachine
	var vi *virtv2.VirtualImage
	var cvi *virtv2.ClusterVirtualImage
	var vdFoo *virtv2.VirtualDisk
	var vdBar *virtv2.VirtualDisk

	blockDeviceHandlerMock := &BlockDeviceServiceMock{}
	blockDeviceHandlerMock.CountBlockDevicesAttachedToVmFunc = func(_ context.Context, vm *virtv2.VirtualMachine) (int, error) {
		return 1, nil
	}

	getBlockDevicesState := func(vi *virtv2.VirtualImage, cvi *virtv2.ClusterVirtualImage, vdFoo, vdBar *virtv2.VirtualDisk) BlockDevicesState {
		return BlockDevicesState{
			VIByName:  map[string]*virtv2.VirtualImage{vi.Name: vi},
			CVIByName: map[string]*virtv2.ClusterVirtualImage{cvi.Name: cvi},
			VDByName:  map[string]*virtv2.VirtualDisk{vdFoo.Name: vdFoo, vdBar.Name: vdBar},
		}
	}

	BeforeEach(func() {
		logger = slog.Default()
		h = NewBlockDeviceHandler(nil, &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}, blockDeviceHandlerMock)
		vi = &virtv2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "vi-01"},
			Status:     virtv2.VirtualImageStatus{Phase: virtv2.ImageReady},
		}
		cvi = &virtv2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "cvi-01"},
			Status:     virtv2.ClusterVirtualImageStatus{Phase: virtv2.ImageReady},
		}
		vdFoo = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-foo"},
			Status: virtv2.VirtualDiskStatus{
				Phase:  virtv2.DiskReady,
				Target: virtv2.DiskTarget{PersistentVolumeClaim: "pvc-foo"},
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
		vdBar = &virtv2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-bar"},
			Status: virtv2.VirtualDiskStatus{
				Phase:  virtv2.DiskReady,
				Target: virtv2.DiskTarget{PersistentVolumeClaim: "pvc-bar"},
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
		vm = &virtv2.VirtualMachine{
			Spec: virtv2.VirtualMachineSpec{
				BlockDeviceRefs: []virtv2.BlockDeviceSpecRef{
					{Name: vi.Name, Kind: virtv2.ImageDevice},
					{Name: cvi.Name, Kind: virtv2.ClusterImageDevice},
					{Name: vdFoo.Name, Kind: virtv2.DiskDevice},
					{Name: vdBar.Name, Kind: virtv2.DiskDevice},
				},
			},
		}
	})

	Context("VirtualMachine is nil", func() {
		It("Not ready, cannot start, no warnings", func() {
			ready, canStart, warnings := h.countReadyBlockDevices(nil, BlockDevicesState{}, logger)
			Expect(ready).To(Equal(0))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})
	})

	Context("BlockDevices are ready", func() {
		It("Ready, can start, no warnings", func() {
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(4))
			Expect(canStart).To(BeTrue())
			Expect(warnings).To(BeNil())
		})
	})

	Context("Image is not ready", func() {
		It("VirtualImage not ready: cannot start, no warnings", func() {
			vi.Status.Phase = virtv2.ImagePending
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})

		It("ClusterVirtualImage not ready: cannot start, no warnings", func() {
			cvi.Status.Phase = virtv2.ImagePending
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})
	})

	Context("Cannot attach VirtualDisk ", func() {
		It("VirtualDisk is attached to different virtual machine", func() {
			vdFoo.Status.AttachedToVirtualMachines = []virtv2.AttachedVirtualMachine{
				{Name: "different"},
			}
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).NotTo(BeEmpty())
		})

		It("VirtualDisk is attached to multiple virtual machines", func() {
			vdFoo.Status.AttachedToVirtualMachines = []virtv2.AttachedVirtualMachine{
				{Name: vm.Name},
				{Name: "different"},
			}
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).ToNot(BeEmpty())
		})
	})

	Context("VirtualDisk is not ready", func() {
		It("VirtualDisk's target pvc is not yet created", func() {
			vdFoo.Status.Phase = virtv2.DiskProvisioning
			vdFoo.Status.Target.PersistentVolumeClaim = ""
			state := getBlockDevicesState(vi, cvi, vdFoo, vdBar)
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeFalse())
			Expect(warnings).To(BeNil())
		})

		It("VirtualDisk's target pvc is created", func() {
			vdFoo.Status.Phase = virtv2.DiskProvisioning
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
			ready, canStart, warnings := h.countReadyBlockDevices(vm, state, logger)
			Expect(ready).To(Equal(3))
			Expect(canStart).To(BeTrue())
			Expect(warnings).ToNot(BeEmpty())
		})
	})
})

var _ = Describe("BlockDeviceHandler Handle", func() {
	Context("Handle call result based on the number of connected block devices", func() {
		okBlockDeviceServiceMock := &BlockDeviceServiceMock{
			CountBlockDevicesAttachedToVmFunc: func(_ context.Context, _ *virtv2.VirtualMachine) (int, error) {
				return 1, nil
			},
		}
		erroredBlockDeviceServiceMock := &BlockDeviceServiceMock{
			CountBlockDevicesAttachedToVmFunc: func(_ context.Context, _ *virtv2.VirtualMachine) (int, error) {
				return 17, nil
			},
		}

		ctx := logger.ToContext(context.TODO(), slog.Default())

		recorderMock := &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}

		scheme := apiruntime.NewScheme()
		for _, f := range []func(*apiruntime.Scheme) error{
			virtv2.AddToScheme,
			virtv1.AddToScheme,
			corev1.AddToScheme,
		} {
			err := f(scheme)
			if err != nil {
				Fail(err.Error())
			}
		}

		namespacedName := types.NamespacedName{
			Namespace: "ns",
			Name:      "vm",
		}

		vm := &virtv2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
			},
			Spec: virtv2.VirtualMachineSpec{},
			Status: virtv2.VirtualMachineStatus{
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

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()
		vmResource := service.NewResource(namespacedName, fakeClient, vmFactoryByVm(vm), vmStatusGetter)
		_ = vmResource.Fetch(ctx)
		vmState := state.New(fakeClient, vmResource)

		It("Should be ok because fewer than 16 devices are connected", func() {
			handler := NewBlockDeviceHandler(fakeClient, recorderMock, okBlockDeviceServiceMock)
			result, err := handler.Handle(ctx, vmState)
			Expect(err).To(BeNil())
			Expect(result).To(Equal(reconcile.Result{}))
			readyCondition, ok := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal(vmcondition.ReasonBlockDevicesReady.String()))
		})
		It("There might be an issue since 16 or more devices are connected.", func() {
			handler := NewBlockDeviceHandler(fakeClient, recorderMock, erroredBlockDeviceServiceMock)
			result, err := handler.Handle(ctx, vmState)
			Expect(err).To(BeNil())
			Expect(result).To(Equal(reconcile.Result{}))
			readyCondition, ok := conditions.GetCondition(vmcondition.TypeBlockDevicesReady, vmState.VirtualMachine().Changed().Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal(vmcondition.ReasonBlockDeviceLimitExceeded.String()))
		})
	})
})

func vmFactoryByVm(vm *virtv2.VirtualMachine) func() *virtv2.VirtualMachine {
	return func() *virtv2.VirtualMachine {
		return vm
	}
}

func vmStatusGetter(obj *virtv2.VirtualMachine) virtv2.VirtualMachineStatus {
	return obj.Status
}
