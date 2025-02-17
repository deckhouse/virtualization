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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
		Expect(virtv2.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())

		ctx = context.TODO()
	})

	Context("when VirtualDisk is not in use", func() {
		It("must set status Unknown and reason Unknown", func() {
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("must set condition generation equal resource generation", func() {
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
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

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
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
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			vm := &virtv2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Spec: virtv2.VirtualMachineSpec{
					BlockDeviceRefs: []virtv2.BlockDeviceSpecRef{
						{
							Kind: virtv2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
				Status: virtv2.VirtualMachineStatus{
					Phase: virtv2.MachineRunning,
					BlockDeviceRefs: []virtv2.BlockDeviceStatusRef{
						{
							Kind: virtv2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, vm).Build()
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

	Context("when VirtualDisk is used by not ready VirtualMachine", func() {
		It("it sets Unknown", func() {
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			vm := &virtv2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Status: virtv2.VirtualMachineStatus{
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
					BlockDeviceRefs: []virtv2.BlockDeviceStatusRef{
						{
							Kind: virtv2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, vm).Build()
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
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			vi := &virtv2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vi",
					Namespace: "default",
				},
				Spec: virtv2.VirtualImageSpec{
					DataSource: virtv2.VirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualImageObjectRef{
							Kind: virtv2.VirtualDiskKind,
							Name: "test-vd",
						},
					},
				},
				Status: virtv2.VirtualImageStatus{
					Phase:      virtv2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, vi).Build()
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

	Context("when VirtualDisk is used by ClusterVirtualImage", func() {
		It("must set status True and reason AllowedForImageUsage", func() {
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			cvi := &virtv2.ClusterVirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vi",
					Namespace: "default",
				},
				Spec: virtv2.ClusterVirtualImageSpec{
					DataSource: virtv2.ClusterVirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.ClusterVirtualImageObjectRef{
							Kind:      virtv2.VirtualDiskKind,
							Name:      "test-vd",
							Namespace: "default",
						},
					},
				},
				Status: virtv2.ClusterVirtualImageStatus{
					Phase:      virtv2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, cvi).Build()
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

	Context("when VirtualDisk is used by VirtualImage and VirtualMachine", func() {
		It("must set status True and reason AllowedForVirtualMachineUsage", func() {
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{},
				},
			}

			vi := &virtv2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vi",
					Namespace: "default",
				},
				Spec: virtv2.VirtualImageSpec{
					DataSource: virtv2.VirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualImageObjectRef{
							Kind: virtv2.VirtualDiskKind,
							Name: "test-vd",
						},
					},
				},
				Status: virtv2.VirtualImageStatus{
					Phase:      virtv2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			vm := &virtv2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Status: virtv2.VirtualMachineStatus{
					BlockDeviceRefs: []virtv2.BlockDeviceStatusRef{
						{
							Kind: virtv2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, vi, vm).Build()
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
			startTime := metav1.Time{Time: time.Now()}
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{
							Type:               vdcondition.InUseType.String(),
							Reason:             vdcondition.UsedForImageCreation.String(),
							Status:             metav1.ConditionTrue,
							LastTransitionTime: startTime,
						},
					},
				},
			}

			vm := &virtv2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Status: virtv2.VirtualMachineStatus{
					BlockDeviceRefs: []virtv2.BlockDeviceStatusRef{
						{
							Kind: virtv2.DiskDevice,
							Name: vd.Name,
						},
					},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, vm).Build()
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.AttachedToVirtualMachine.String()))
			Expect(cond.LastTransitionTime).ToNot(Equal(startTime))
		})
	})

	Context("when VirtualDisk is used by VirtualImage after running VM", func() {
		It("must set status True and reason AllowedForImageUsage", func() {
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
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

			vi := &virtv2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vi",
					Namespace: "default",
				},
				Spec: virtv2.VirtualImageSpec{
					DataSource: virtv2.VirtualImageDataSource{
						Type: virtv2.DataSourceTypeObjectRef,
						ObjectRef: &virtv2.VirtualImageObjectRef{
							Kind: virtv2.VirtualDiskKind,
							Name: "test-vd",
						},
					},
				},
				Status: virtv2.VirtualImageStatus{
					Phase:      virtv2.ImageProvisioning,
					Conditions: []metav1.Condition{},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd, vi).Build()
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
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
				Status: virtv2.VirtualDiskStatus{
					Conditions: []metav1.Condition{
						{
							Type:   vdcondition.InUseType.String(),
							Reason: vdcondition.UsedForImageCreation.String(),
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
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
			vd := &virtv2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vd",
					Namespace: "default",
				},
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

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vd).Build()
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
})
