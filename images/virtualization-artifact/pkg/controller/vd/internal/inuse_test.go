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
	virtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, vm, vm2, vm3)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, vm)
			Expect(err).ToNot(HaveOccurred())

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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd)
			Expect(err).ToNot(HaveOccurred())

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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, vm)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, vm)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, vi)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, cvi)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, vi, vm)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, vm)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, vi)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd)
			Expect(err).ToNot(HaveOccurred())
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

			k8sClient, err := testutil.NewFakeClientWithObjects(vd)
			Expect(err).ToNot(HaveOccurred())
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
	Context("when VirtualDisk is used by DataExport", func() {
		It("must set status True and reason UsedForDataExport", func() {
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
						annotations.AnnDataExportRequest: "true",
					},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			}

			k8sClient, err := testutil.NewFakeClientWithObjects(vd, pvc)
			Expect(err).ToNot(HaveOccurred())
			handler = NewInUseHandler(k8sClient)

			result, err := handler.Handle(ctx, vd)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			cond, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.UsedForDataExport.String()))
		})
	})
})
