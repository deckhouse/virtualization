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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/pcidevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/pcidevicecondition"
)

var _ = Describe("DeletionHandler", func() {
	var ctx context.Context

	type testCase struct {
		deleting         bool
		attached         bool
		withVM           bool
		withMultipleVMs  bool
		expectRequeue    bool
		finalizerPresent bool
		expectError      bool
		vmNotFound       bool
	}

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	DescribeTable("Handle",
		func(tc testCase) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			now := metav1.Now()
			pci := &v1alpha2.PCIDevice{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pci-device-1",
					Namespace:         "default",
					DeletionTimestamp: &now,
					Finalizers:        []string{v1alpha2.FinalizerPCIDeviceCleanup},
				},
			}
			if !tc.deleting {
				pci.DeletionTimestamp = nil
				pci.Finalizers = nil
			}

			condStatus := metav1.ConditionFalse
			condReason := string(pcidevicecondition.Available)
			if tc.attached {
				condStatus = metav1.ConditionTrue
				condReason = string(pcidevicecondition.AttachedToVirtualMachine)
			}
			pci.Status.Conditions = []metav1.Condition{{Type: string(pcidevicecondition.AttachedType), Status: condStatus, Reason: condReason}}

			objects := []client.Object{pci}

			vms := make([]client.Object, 0)
			if tc.withVM {
				vm := &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{PCIDevices: []v1alpha2.PCIDeviceSpecRef{{Name: "pci-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{PCIDevices: []v1alpha2.PCIDeviceStatusRef{{Name: "pci-device-1", Attached: true}}},
				}
				if tc.vmNotFound {
					vm = nil
				}
				if vm != nil {
					vms = append(vms, vm)
				}
			}

			if tc.withMultipleVMs {
				vms = append(vms, &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "test-vm-2", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{PCIDevices: []v1alpha2.PCIDeviceSpecRef{{Name: "pci-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{PCIDevices: []v1alpha2.PCIDeviceStatusRef{{Name: "pci-device-1", Attached: true}}},
				})
			}

			objects = append(objects, vms...)

			vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithIndex(vmObj, vmField, vmExtractValue).Build()

			res := reconciler.NewResource(
				types.NamespacedName{Name: pci.Name, Namespace: pci.Namespace},
				cl,
				func() *v1alpha2.PCIDevice { return &v1alpha2.PCIDevice{} },
				func(obj *v1alpha2.PCIDevice) v1alpha2.PCIDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			st := state.New(cl, res)
			h := NewDeletionHandler()
			result, err := h.Handle(ctx, st)

			if tc.expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			if tc.expectRequeue {
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			} else {
				Expect(result).To(Equal(reconcile.Result{}))
			}

			if tc.finalizerPresent {
				Expect(res.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerPCIDeviceCleanup))
			} else {
				Expect(res.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerPCIDeviceCleanup))
			}
		},
		Entry("not deleting adds finalizer", testCase{finalizerPresent: true}),
		Entry("deleting not attached removes finalizer", testCase{deleting: true, finalizerPresent: false}),
		Entry("deleting attached requeues", testCase{deleting: true, attached: true, withVM: true, expectRequeue: true, finalizerPresent: true}),
		Entry("deleting with multiple VMs requeues", testCase{deleting: true, attached: true, withMultipleVMs: true, expectRequeue: true, finalizerPresent: true}),
		Entry("deleting with VM not found removes finalizer", testCase{deleting: true, attached: true, withVM: true, vmNotFound: true, finalizerPresent: false}),
	)
})
