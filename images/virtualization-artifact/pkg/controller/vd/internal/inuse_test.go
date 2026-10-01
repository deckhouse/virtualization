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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("InUseHandler", func() {
	var (
		scheme  *runtime.Scheme
		ctx     context.Context
		handler *InUseHandler
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())

		ctx = context.TODO()
	})

	Context("when handling VirtualDisk usage", func() {
		It("should correctly update status for a disk used by a running VM", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{
							Name:    "test-vm",
							Mounted: false,
						},
						{
							Name:    "test-vm2",
							Mounted: true,
						},
						{
							Name:    "test-vm3",
							Mounted: false,
						},
					},
				},
			}

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "test-vd",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "test-vd",
						},
					},
				},
			}

			vm2 := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm2",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "test-vd",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "test-vd",
						},
					},
				},
			}

			vm3 := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm3",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "test-vd",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "test-vd",
						},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, vm, vm2, vm3).Build()
			handler = &InUseHandler{client: k8sClient}

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.AttachedToVirtualMachine.String()))

			Expect(len(vd.Status.AttachedToVirtualMachines)).To(Equal(3))

			found := false
			for _, attachedVM := range vd.Status.AttachedToVirtualMachines {
				if attachedVM.Name == "test-vm2" && attachedVM.Mounted {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Expected to find 'test-vm' with Mounted true in AttachedToVirtualMachines")
		})

		It("should correctly update status for a disk used by a stopped VM", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{
							Name:    "test-vm",
							Mounted: true,
						},
					},
				},
			}

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "test-vd",
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineStopped,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: "test-vd",
						},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, vm).Build()
			handler = &InUseHandler{client: k8sClient}

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vdcondition.NotInUse.String()))

			Expect(len(vd.Status.AttachedToVirtualMachines)).To(Equal(1))
			Expect(vd.Status.AttachedToVirtualMachines[0].Name).To(Equal("test-vm"))
			Expect(vd.Status.AttachedToVirtualMachines[0].Mounted).To(BeFalse())
		})

		It("should update the status to NotInUse if no VM uses the disk", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd).Build()
			handler = &InUseHandler{client: k8sClient}

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vdcondition.NotInUse.String()))

			Expect(len(vd.Status.AttachedToVirtualMachines)).To(Equal(0))
		})

		It("should handle VM disappearance and update status accordingly", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
					AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
						{Name: "missing-vm", Mounted: true},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd).Build()
			handler = &InUseHandler{client: k8sClient}

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vdcondition.NotInUse.String()))

			Expect(len(vd.Status.AttachedToVirtualMachines)).To(Equal(0))
		})
	})

	Context("when VirtualDisk is not in use", func() {
		It("must set status Unknown and reason Unknown", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("must set condition generation equal resource generation", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{
							Type:               vdcondition.InUseType.String(),
							Reason:             conditions.ReasonUnknown.String(),
							Status:             metav1.ConditionUnknown,
							ObservedGeneration: 2,
						},
					},
				},
			}
			vd.Generation = 3

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.ObservedGeneration).To(Equal(vd.Generation))
		})
	})

	Context("when VirtualDisk is used by running VirtualMachine", func() {
		It("must set status True and reason AllowedForVirtualMachineUsage", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualMachineSpec{
					BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, vm).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.AttachedToVirtualMachine.String()))
			Expect(cond.Message).To(Equal(`The VirtualDisk is in use by the VirtualMachine "test-vm"; detach it or stop the VirtualMachine to release the VirtualDisk.`))
		})
	})

	Context("when VirtualDisk is used by not ready VirtualMachine", func() {
		It("it sets Unknown", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualMachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:   vmcondition.TypeMigrating.String(),
							Status: metav1.ConditionFalse,
						},
						{
							Type:   vmcondition.TypeIPAddressReady.String(),
							Status: metav1.ConditionFalse,
						},
					},
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, vm).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("when VirtualDisk is used by VirtualImage", func() {
		It("must set status True and reason AllowedForImageUsage", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Phase:      v1alpha2.DiskReady,
					Conditions: []metav1.Condition{},
				},
			}

			vi := &v1alpha2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vi",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualImageSpec{
					DataSource: v1alpha2.VirtualImageDataSource{
						Type: v1alpha2.DataSourceTypeObjectRef,
						ObjectRef: &v1alpha2.VirtualImageObjectRef{
							Kind: v1alpha2.VirtualDiskKind,
							Name: "test-vd",
						},
					},
				},
				Status: v1alpha2.VirtualImageStatus{
					Phase:      v1alpha2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, vi).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.UsedForImageCreation.String()))
			Expect(cond.Message).To(Equal(`The VirtualDisk is in use for creating the VirtualImage "test-vi"; the creation must finish to release the disk.`))
		})
	})

	Context("when VirtualDisk is used by ClusterVirtualImage", func() {
		It("must set status True and reason AllowedForImageUsage", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Phase:      v1alpha2.DiskReady,
					Conditions: []metav1.Condition{},
				},
			}

			cvi := &v1alpha2.ClusterVirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vi",
					Namespace: "default",
				},
				Spec: v1alpha2.ClusterVirtualImageSpec{
					DataSource: v1alpha2.ClusterVirtualImageDataSource{
						Type: v1alpha2.DataSourceTypeObjectRef,
						ObjectRef: &v1alpha2.ClusterVirtualImageObjectRef{
							Kind:      v1alpha2.VirtualDiskKind,
							Name:      "test-vd",
							Namespace: "default",
						},
					},
				},
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase:      v1alpha2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, cvi).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.UsedForImageCreation.String()))
			Expect(cond.Message).To(Equal(`The VirtualDisk is in use for creating the ClusterVirtualImage "test-vi"; the creation must finish to release the disk.`))
		})

		It("is not taken by a ClusterVirtualImage created from a disk with the same name in another namespace", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Phase:      v1alpha2.DiskReady,
					Conditions: []metav1.Condition{},
				},
			}

			cvi := &v1alpha2.ClusterVirtualImage{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vi"},
				Spec: v1alpha2.ClusterVirtualImageSpec{
					DataSource: v1alpha2.ClusterVirtualImageDataSource{
						Type: v1alpha2.DataSourceTypeObjectRef,
						ObjectRef: &v1alpha2.ClusterVirtualImageObjectRef{
							Kind:      v1alpha2.VirtualDiskKind,
							Name:      "test-vd",
							Namespace: "other",
						},
					},
				},
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase:      v1alpha2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, cvi).Build()
			handler = NewInUseHandler(k8sClient)

			_, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond.Reason).ToNot(Equal(vdcondition.UsedForImageCreation.String()))
		})
	})

	Context("when VirtualDisk is used by VirtualImage and VirtualMachine", func() {
		It("must set status True and reason AllowedForVirtualMachineUsage", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			vi := &v1alpha2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vi",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualImageSpec{
					DataSource: v1alpha2.VirtualImageDataSource{
						Type: v1alpha2.DataSourceTypeObjectRef,
						ObjectRef: &v1alpha2.VirtualImageObjectRef{
							Kind: v1alpha2.VirtualDiskKind,
							Name: "test-vd",
						},
					},
				},
				Status: v1alpha2.VirtualImageStatus{
					Phase:      v1alpha2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineStarting,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, vi, vm).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.AttachedToVirtualMachine.String()))
		})
	})

	Context("when VirtualDisk is used by VirtualMachine after create image", func() {
		It("must set status True and reason AllowedForVirtualMachineUsage", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{
							Type:   vdcondition.InUseType.String(),
							Reason: vdcondition.UsedForImageCreation.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachinePending,
					BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
						{
							Kind: v1alpha2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, vm).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.AttachedToVirtualMachine.String()))
		})
	})

	Context("when VirtualDisk is used by VirtualImage after running VM", func() {
		It("must set status True and reason AllowedForImageUsage", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Phase: v1alpha2.DiskReady,
					Conditions: []metav1.Condition{
						{
							Type:   vdcondition.InUseType.String(),
							Reason: vdcondition.AttachedToVirtualMachine.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			vi := &v1alpha2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vi",
					Namespace: "default",
				},
				Spec: v1alpha2.VirtualImageSpec{
					DataSource: v1alpha2.VirtualImageDataSource{
						Type: v1alpha2.DataSourceTypeObjectRef,
						ObjectRef: &v1alpha2.VirtualImageObjectRef{
							Kind: v1alpha2.VirtualDiskKind,
							Name: "test-vd",
						},
					},
				},
				Status: v1alpha2.VirtualImageStatus{
					Phase:      v1alpha2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, vi).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.UsedForImageCreation.String()))
		})
	})

	Context("when VirtualDisk is not in use after image creation", func() {
		It("must set status False and reason NotInUse", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{
							Type:   vdcondition.InUseType.String(),
							Reason: vdcondition.UsedForImageCreation.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vdcondition.NotInUse.String()))
		})
	})

	Context("when VirtualDisk is not in use after VM deletion", func() {
		It("must set status False and reason NotInUse", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{
							Type:   vdcondition.InUseType.String(),
							Reason: vdcondition.AttachedToVirtualMachine.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vdcondition.NotInUse.String()))
		})
	})
	Context("when virtual machines claim the disk through the spec and through attachments", func() {
		const vdName = "shared-vd"

		vm := func(name string, phase v1alpha2.MachinePhase, listed bool) *v1alpha2.VirtualMachine {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Status:     v1alpha2.VirtualMachineStatus{Phase: phase},
			}
			if listed {
				vm.Status.BlockDeviceRefs = []v1alpha2.BlockDeviceStatusRef{{Kind: v1alpha2.DiskDevice, Name: vdName}}
			}
			return vm
		}
		// attachmentOfB returns an attachment of the disk to vm-b.
		attachmentOfB := func(phase v1alpha2.BlockDeviceAttachmentPhase) *v1alpha2.VirtualMachineBlockDeviceAttachment {
			return &v1alpha2.VirtualMachineBlockDeviceAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "vmbda-b", Namespace: "default"},
				Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
					VirtualMachineName: "vm-b",
					BlockDeviceRef:     v1alpha2.VMBDAObjectRef{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: vdName},
				},
				Status: v1alpha2.VirtualMachineBlockDeviceAttachmentStatus{Phase: phase},
			}
		}
		// instance returns the internal instance of a VM that still has the volume of the disk.
		instance := func(vmName string, phase virtv1.VolumePhase) *virtv1.VirtualMachineInstance {
			return &virtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: "default"},
				Status: virtv1.VirtualMachineInstanceStatus{
					VolumeStatus: []virtv1.VolumeStatus{{Name: "vd-" + vdName, Phase: phase, HotplugVolume: &virtv1.HotplugVolumeStatus{}}},
				},
			}
		}

		handle := func(current string, objs ...client.Object) []v1alpha2.AttachedVirtualMachine {
			GinkgoHelper()
			vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: vdName, Namespace: "default"}}
			if current != "" {
				vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: current, Mounted: true}}
			}
			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(append(objs, vd)...).Build()
			_, err := NewInUseHandler(k8sClient).Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			return vd.Status.AttachedToVirtualMachines
		}

		It("gives the disk to one of them and keeps a waiting attachment out of the list", func() {
			attached := handle("",
				vm("vm-a", v1alpha2.MachineRunning, true),
				vm("vm-b", v1alpha2.MachineRunning, false),
				attachmentOfB(v1alpha2.BlockDeviceAttachmentPhasePending),
			)
			Expect(attached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a", Mounted: true}}))
		})

		It("gives the disk to the attachment when no running VM claims it through the spec", func() {
			attached := handle("",
				vm("vm-a", v1alpha2.MachineStopped, true),
				vm("vm-b", v1alpha2.MachineRunning, false),
				attachmentOfB(v1alpha2.BlockDeviceAttachmentPhasePending),
			)
			Expect(attached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a"}, {Name: "vm-b", Mounted: true}}))
		})

		It("keeps the disk with the attachment owner against a VM that adds it to the spec later", func() {
			attached := handle("vm-b",
				vm("vm-a", v1alpha2.MachineRunning, true),
				vm("vm-b", v1alpha2.MachineRunning, false),
				attachmentOfB(v1alpha2.BlockDeviceAttachmentPhasePending),
			)
			Expect(attached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a"}, {Name: "vm-b", Mounted: true}}))
		})

		DescribeTable("does not count an attachment that never plugs the disk",
			func(att *v1alpha2.VirtualMachineBlockDeviceAttachment) {
				attached := handle("", vm("vm-b", v1alpha2.MachineRunning, false), att)
				Expect(attached).To(BeEmpty())
			},
			Entry("conflicting attachment", attachmentOfB(v1alpha2.BlockDeviceAttachmentPhaseFailed)),
			Entry("deleted attachment", func() *v1alpha2.VirtualMachineBlockDeviceAttachment {
				att := attachmentOfB(v1alpha2.BlockDeviceAttachmentPhaseAttached)
				att.DeletionTimestamp = ptr.To(metav1.Now())
				att.Finalizers = []string{"test"}
				return att
			}()),
		)

		It("keeps the disk with a VM that removed it from the spec until the volume leaves its instance", func() {
			attached := handle("vm-a",
				vm("vm-a", v1alpha2.MachineRunning, false), instance("vm-a", virtv1.HotplugVolumeDetaching),
				vm("vm-b", v1alpha2.MachineRunning, true),
			)
			Expect(attached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a", Mounted: true}, {Name: "vm-b"}}))
		})

		It("names every VM plugged into the disk after an upgrade and takes it from none of them", func() {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: vdName, Namespace: "default"},
				Status:     v1alpha2.VirtualDiskStatus{AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{{Name: "vm-b", Mounted: true}}},
			}
			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd,
				vm("vm-a", v1alpha2.MachineRunning, true), instance("vm-a", virtv1.VolumeReady),
				vm("vm-b", v1alpha2.MachineRunning, true), instance("vm-b", virtv1.VolumeReady),
			).Build()

			_, err := NewInUseHandler(k8sClient).Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())

			Expect(vd.Status.AttachedToVirtualMachines).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a"}, {Name: "vm-b", Mounted: true}}))
			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond.Message).To(ContainSubstring(`in use by 2 VirtualMachines`))
			Expect(cond.Message).To(ContainSubstring(`"vm-a", "vm-b"`))
		})

		It("keeps the disk with the old node while the DVP CSI driver moves it to another node", func() {
			// The CSI driver deletes the attachment to the old node and creates one to the new node at once,
			// before KubeVirt has detached the volume from the old node.
			oldAttachment := &v1alpha2.VirtualMachineBlockDeviceAttachment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vmbda-" + vdName + "-vm-a", Namespace: "default",
					DeletionTimestamp: ptr.To(metav1.Now()), Finalizers: []string{"test"},
				},
				Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
					VirtualMachineName: "vm-a",
					BlockDeviceRef:     v1alpha2.VMBDAObjectRef{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: vdName},
				},
				Status: v1alpha2.VirtualMachineBlockDeviceAttachmentStatus{Phase: v1alpha2.BlockDeviceAttachmentPhaseAttached},
			}
			detaching := handle("vm-a",
				vm("vm-a", v1alpha2.MachineRunning, true), instance("vm-a", virtv1.HotplugVolumeDetaching), oldAttachment,
				vm("vm-b", v1alpha2.MachineRunning, false), attachmentOfB(v1alpha2.BlockDeviceAttachmentPhasePending),
			)
			Expect(detaching).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a", Mounted: true}}))

			detached := handle("vm-a",
				vm("vm-a", v1alpha2.MachineRunning, false), &virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Name: "vm-a", Namespace: "default"}},
				vm("vm-b", v1alpha2.MachineRunning, false), attachmentOfB(v1alpha2.BlockDeviceAttachmentPhasePending),
			)
			Expect(detached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-b", Mounted: true}}))
		})

		It("keeps the disk with a VM whose instance took the volume before it shows in the volume status", func() {
			// KubeVirt drops the attach request once the volume is in the internal VM template, and the volume can
			// leave the template again before the instance reports it: only the instance spec still has it then.
			kvvmi := &virtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "vm-a", Namespace: "default"},
				Spec:       virtv1.VirtualMachineInstanceSpec{Volumes: []virtv1.Volume{{Name: "vd-" + vdName}}},
			}
			attached := handle("vm-a",
				vm("vm-a", v1alpha2.MachineRunning, false), kvvmi,
				vm("vm-b", v1alpha2.MachineRunning, false), attachmentOfB(v1alpha2.BlockDeviceAttachmentPhasePending),
			)
			Expect(attached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a", Mounted: true}}))
		})

		It("does not give the disk to a waiting attachment while an image is being created from it", func() {
			disk := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: vdName, Namespace: "default"},
				Status:     v1alpha2.VirtualDiskStatus{Phase: v1alpha2.DiskReady},
			}
			image := &v1alpha2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{Name: "vi-from-disk", Namespace: "default"},
				Spec: v1alpha2.VirtualImageSpec{DataSource: v1alpha2.VirtualImageDataSource{
					Type:      v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualImageObjectRef{Kind: v1alpha2.VirtualImageObjectRefKindVirtualDisk, Name: vdName},
				}},
				Status: v1alpha2.VirtualImageStatus{Phase: v1alpha2.ImageProvisioning},
			}
			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(disk, image,
				vm("vm-b", v1alpha2.MachineRunning, false), attachmentOfB(v1alpha2.BlockDeviceAttachmentPhasePending),
			).Build()

			_, err := NewInUseHandler(k8sClient).Handle(ctx, disk)
			Expect(err).ToNot(HaveOccurred())

			Expect(disk.Status.AttachedToVirtualMachines).To(BeEmpty())
			cond, _ := conditions.GetCondition(vdcondition.InUseType, disk.Status.Conditions)
			Expect(cond.Reason).To(Equal(vdcondition.UsedForImageCreation.String()))
		})

		It("gives the disk away once the volume has left the instance", func() {
			attached := handle("vm-a",
				vm("vm-a", v1alpha2.MachineRunning, false), &virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Name: "vm-a", Namespace: "default"}},
				vm("vm-b", v1alpha2.MachineRunning, true),
			)
			Expect(attached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-b", Mounted: true}}))
		})

		It("counts a pending attach request as holding the disk", func() {
			kvvm := &virtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm-b", Namespace: "default"},
				Status: virtv1.VirtualMachineStatus{VolumeRequests: []virtv1.VirtualMachineVolumeRequest{
					{AddVolumeOptions: &virtv1.AddVolumeOptions{Name: "vd-" + vdName}},
				}},
			}
			// The attachment behind the request is already deleted, but the volume still reaches vm-b.
			attached := handle("vm-b",
				vm("vm-a", v1alpha2.MachineRunning, true),
				vm("vm-b", v1alpha2.MachineRunning, false), kvvm,
				&virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Name: "vm-b", Namespace: "default"}},
			)
			Expect(attached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a"}, {Name: "vm-b", Mounted: true}}))
		})

		It("does not let a VM recreated under the name of the owner inherit the disk it no longer holds", func() {
			// The old vm-a is gone with its instance; the new vm-a is stopped and only refers to the disk.
			attached := handle("vm-a",
				vm("vm-a", v1alpha2.MachineStopped, true),
				vm("vm-b", v1alpha2.MachineRunning, true),
			)
			Expect(attached).To(Equal([]v1alpha2.AttachedVirtualMachine{{Name: "vm-a"}, {Name: "vm-b", Mounted: true}}))
		})
	})

	DescribeTable("electOwner",
		func(current string, candidates []ownerCandidate, owner string) {
			Expect(electOwner(current, candidates)).To(Equal(owner))
		},
		Entry("nobody claims the disk", "", nil, ""),
		Entry("only inactive claimants", "", []ownerCandidate{{name: "a", listed: true}}, ""),
		Entry("the first active claimant by name", "", []ownerCandidate{{name: "b", active: true}, {name: "a", active: true}}, "a"),
		Entry("the current owner among active claimants", "b", []ownerCandidate{{name: "a", active: true}, {name: "b", active: true}}, "b"),
		Entry("a holder over the current owner that only claims", "b", []ownerCandidate{{name: "a", holds: true}, {name: "b", active: true}}, "a"),
		Entry("the current owner among holders", "b", []ownerCandidate{{name: "a", holds: true}, {name: "b", holds: true}}, "b"),
		Entry("the first holder by name without an owner", "", []ownerCandidate{{name: "b", holds: true}, {name: "a", holds: true}}, "a"),
		Entry("an owner that neither holds nor is active", "a", []ownerCandidate{{name: "a", listed: true}, {name: "b", active: true}}, "b"),
	)

	Context("when VirtualDisk is used by DataExport", func() {
		DescribeTable("must set status True and reason UsedForDataExport", func(annotationKey string) {
			vd := &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: v1alpha2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
					Target: v1alpha2.DiskTarget{
						PersistentVolumeClaim: "test-pvc",
					},
				},
			}
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					Annotations: map[string]string{
						annotationKey: "true",
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			}

			k8sClient := withVolumeIndexes(fake.NewClientBuilder().WithScheme(scheme)).WithObjects(vd, pvc).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.UsedForDataExport.String()))
			Expect(cond.Message).To(Equal("The VirtualDisk is in use by a data export request; the export must finish to release the disk."))
		},
			Entry("annotation of the storage-foundation module", annotations.AnnDataExportRequest),
			Entry("annotation of the storage module before the rename", annotations.AnnDataExportRequestLegacy),
		)
	})
})

// withVolumeIndexes registers the indexes the owner election looks the volume holders up by.
func withVolumeIndexes(b *fake.ClientBuilder) *fake.ClientBuilder {
	for _, index := range []indexer.IndexGetter{indexer.IndexKVVMIByVolume, indexer.IndexKVVMByAddVolumeRequest} {
		b.WithIndex(index())
	}
	return b
}
