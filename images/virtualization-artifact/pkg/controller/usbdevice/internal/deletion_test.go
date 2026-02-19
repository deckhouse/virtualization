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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	fakeversioned "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/fake"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

var _ = Describe("DeletionHandler", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	DescribeTable("Handle",
		func(deleting, attached, withVM, expectFinalizerPresent, expectRequeue bool) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			usb := &v1alpha2.USBDevice{ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default"}}
			if deleting {
				now := metav1.Now()
				usb.DeletionTimestamp = &now
				usb.Finalizers = []string{v1alpha2.FinalizerUSBDeviceCleanup}
			}
			condStatus := metav1.ConditionFalse
			condReason := string(usbdevicecondition.Available)
			if attached {
				condStatus = metav1.ConditionTrue
				condReason = string(usbdevicecondition.AttachedToVirtualMachine)
			}
			usb.Status.Conditions = []metav1.Condition{{Type: string(usbdevicecondition.AttachedType), Status: condStatus, Reason: condReason}}

			objects := []client.Object{usb}
			if withVM {
				objects = append(objects, &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}}},
				})
			}

			vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
			nodeObj, nodeField, nodeExtractValue := indexer.IndexNodeUSBDeviceByName()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithIndex(vmObj, vmField, vmExtractValue).WithIndex(nodeObj, nodeField, nodeExtractValue).Build()

			res := reconciler.NewResource(
				types.NamespacedName{Name: usb.Name, Namespace: usb.Namespace},
				cl,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			st := state.New(cl, res)
			h := NewDeletionHandler(cl, fakeversioned.NewSimpleClientset(), &eventrecord.EventRecorderLoggerMock{EventfFunc: func(client.Object, string, string, string, ...any) {}})
			result, err := h.Handle(ctx, st)
			Expect(err).NotTo(HaveOccurred())

			if expectRequeue {
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			} else {
				Expect(result).To(Equal(reconcile.Result{}))
			}

			if expectFinalizerPresent {
				Expect(res.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerUSBDeviceCleanup))
			} else {
				Expect(res.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerUSBDeviceCleanup))
			}
		},
		Entry("not deleting adds finalizer", false, false, false, true, false),
		Entry("deleting not attached removes finalizer", true, false, false, false, false),
		Entry("deleting attached requeues", true, true, true, true, true),
	)
})
