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
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
)

var _ = Describe("LifecycleHandler", func() {
	var ctx context.Context
	var scheme *apiruntime.Scheme

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
		scheme = apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(resourcev1.AddToScheme(scheme)).To(Succeed())
	})

	DescribeTable("Handle",
		func(hasNode bool, nodeConditionReason string, withVM, withMultipleVMs bool, expectReady metav1.ConditionStatus, expectReadyReason, expectAttachedReason string) {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default", UID: "usb-uid-1"},
				Status:     v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-device-1", VendorID: "0000", ProductID: "0000"}},
			}

			objects := []client.Object{usbDevice}
			if hasNode {
				objects = append(objects, &v1alpha2.NodeUSBDevice{
					ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1"},
					Status: v1alpha2.NodeUSBDeviceStatus{
						Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-device-1", VendorID: "1234", ProductID: "5678"},
						NodeName:   "node-1",
						Conditions: []metav1.Condition{{Type: string(nodeusbdevicecondition.ReadyType), Reason: nodeConditionReason, Message: "Node status"}},
					},
				})
			}

			if withVM {
				vm := &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "vm-1", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}}},
				}
				objects = append(objects, vm)
			}

			if withMultipleVMs {
				objects = append(objects, &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "vm-2", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}}},
				})
			}

			vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
			vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithIndex(vmObj, vmField, vmExtractValue).WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).Build()

			res := reconciler.NewResource(
				types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
				cl,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			st := state.New(cl, res)
			h := NewLifecycleHandler(cl)
			_, err := h.Handle(ctx, st)
			Expect(err).NotTo(HaveOccurred())

			ready := meta.FindStatusCondition(res.Changed().Status.Conditions, string(usbdevicecondition.ReadyType))
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(expectReady))
			Expect(ready.Reason).To(Equal(expectReadyReason))

			attached := meta.FindStatusCondition(res.Changed().Status.Conditions, string(usbdevicecondition.AttachedType))
			Expect(attached).NotTo(BeNil())
			Expect(attached.Reason).To(Equal(expectAttachedReason))

			template := &resourcev1.ResourceClaimTemplate{}
			err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("usb-device-1"), Namespace: "default"}, template)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("node ready and not attached", true, string(nodeusbdevicecondition.Ready), false, false, metav1.ConditionTrue, string(usbdevicecondition.Ready), string(usbdevicecondition.Available)),
		Entry("node ready and attached to one VM", true, string(nodeusbdevicecondition.Ready), true, false, metav1.ConditionTrue, string(usbdevicecondition.Ready), string(usbdevicecondition.AttachedToVirtualMachine)),
		Entry("node ready and attached to multiple VMs", true, string(nodeusbdevicecondition.Ready), true, true, metav1.ConditionTrue, string(usbdevicecondition.Ready), string(usbdevicecondition.AttachedToVirtualMachine)),
		Entry("node not ready", true, string(nodeusbdevicecondition.NotReady), false, false, metav1.ConditionFalse, string(usbdevicecondition.NotReady), string(usbdevicecondition.Available)),
		Entry("node not found", true, string(nodeusbdevicecondition.NotFound), false, false, metav1.ConditionFalse, string(usbdevicecondition.NotFound), string(usbdevicecondition.Available)),
		Entry("node missing", false, "", false, false, metav1.ConditionFalse, string(usbdevicecondition.NotFound), string(usbdevicecondition.Available)),
	)

	It("should set NoFreeUSBIPPort when VM references device but attach cannot start", func() {
		usbDevice := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default", UID: "usb-uid-1"},
			Status: v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{
				Name:      "usb-device-1",
				VendorID:  "1234",
				ProductID: "5678",
				Speed:     480,
			}},
		}

		nodeUSBDevice := &v1alpha2.NodeUSBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1"},
			Status: v1alpha2.NodeUSBDeviceStatus{
				Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-device-1", VendorID: "1234", ProductID: "5678", Speed: 480},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodeusbdevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodeusbdevicecondition.Ready), Message: "Node status"}},
			},
		}

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-1", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
			Status: v1alpha2.VirtualMachineStatus{
				Node:       "node-2",
				USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: false}},
			},
		}

		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Annotations: map[string]string{
			annotations.AnnUSBIPTotalPorts:            "2",
			annotations.AnnUSBIPHighSpeedHubUsedPorts: "1",
		}}}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
		vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice, nodeUSBDevice, vm, node).WithIndex(vmObj, vmField, vmExtractValue).WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
			cl,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		attached := meta.FindStatusCondition(res.Changed().Status.Conditions, string(usbdevicecondition.AttachedType))
		Expect(attached).NotTo(BeNil())
		Expect(attached.Reason).To(Equal(string(usbdevicecondition.NoFreeUSBIPPort)))
		Expect(attached.Status).To(Equal(metav1.ConditionFalse))
		Expect(attached.Message).To(ContainSubstring("requested by VirtualMachine"))
	})

	It("should keep AttachedToVirtualMachine when at least one VM already has device attached", func() {
		usbDevice := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default", UID: "usb-uid-1"},
			Status: v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{
				Name:      "usb-device-1",
				VendorID:  "1234",
				ProductID: "5678",
				Speed:     480,
			}},
		}

		nodeUSBDevice := &v1alpha2.NodeUSBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1"},
			Status: v1alpha2.NodeUSBDeviceStatus{
				Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-device-1", VendorID: "1234", ProductID: "5678", Speed: 480},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodeusbdevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodeusbdevicecondition.Ready), Message: "Node status"}},
			},
		}

		vmAttached := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-attached", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
			Status: v1alpha2.VirtualMachineStatus{
				Node:       "node-2",
				USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: true}},
			},
		}

		vmPending := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-pending", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
			Status: v1alpha2.VirtualMachineStatus{
				Node:       "node-2",
				USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: false}},
			},
		}

		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Annotations: map[string]string{
			annotations.AnnUSBIPTotalPorts:            "2",
			annotations.AnnUSBIPHighSpeedHubUsedPorts: "1",
		}}}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
		vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice, nodeUSBDevice, vmAttached, vmPending, node).WithIndex(vmObj, vmField, vmExtractValue).WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
			cl,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		attached := meta.FindStatusCondition(res.Changed().Status.Conditions, string(usbdevicecondition.AttachedType))
		Expect(attached).NotTo(BeNil())
		Expect(attached.Reason).To(Equal(string(usbdevicecondition.AttachedToVirtualMachine)))
		Expect(attached.Status).To(Equal(metav1.ConditionTrue))
		Expect(attached.Message).To(ContainSubstring("attached to"))
	})

	It("should return error when USBIP port availability check fails unexpectedly", func() {
		usbDevice := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default", UID: "usb-uid-1"},
			Status: v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{
				Name:      "usb-device-1",
				VendorID:  "1234",
				ProductID: "5678",
				Speed:     480,
			}},
		}

		nodeUSBDevice := &v1alpha2.NodeUSBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1"},
			Status: v1alpha2.NodeUSBDeviceStatus{
				Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-device-1", VendorID: "1234", ProductID: "5678", Speed: 480},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodeusbdevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodeusbdevicecondition.Ready), Message: "Node status"}},
			},
		}

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-1", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineSpec{USBDevices: []v1alpha2.USBDeviceSpecRef{{Name: "usb-device-1"}}},
			Status:     v1alpha2.VirtualMachineStatus{Node: "node-2", USBDevices: []v1alpha2.USBDeviceStatusRef{{Name: "usb-device-1", Attached: false}}},
		}

		badNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Annotations: map[string]string{
			annotations.AnnUSBIPTotalPorts:             "invalid",
			annotations.AnnUSBIPHighSpeedHubUsedPorts:  "0",
			annotations.AnnUSBIPSuperSpeedHubUsedPorts: "0",
		}}}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
		vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice, nodeUSBDevice, vm, badNode).WithIndex(vmObj, vmField, vmExtractValue).WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
			cl,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to check free USBIP ports for USBDevice"))
	})

	It("should skip ResourceClaimTemplate when attribute name is empty", func() {
		usbDevice := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-1", Namespace: "default", UID: "usb-uid-1"},
			Status:     v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "", VendorID: "0000", ProductID: "0000"}},
		}

		objects := []client.Object{usbDevice}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
		vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithIndex(vmObj, vmField, vmExtractValue).WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
			cl,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		template := &resourcev1.ResourceClaimTemplate{}
		err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("usb-device-1"), Namespace: "default"}, template)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	DescribeTable("ResourceClaimTemplate request and selector names",
		func(attrName, expectedSelectorName string) {
			usbDevice := &v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-cr", Namespace: "default", UID: "usb-uid-1"},
				Status:     v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: attrName, VendorID: "0000", ProductID: "0000"}},
			}

			nodeUSBDevice := &v1alpha2.NodeUSBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-cr"},
				Status: v1alpha2.NodeUSBDeviceStatus{
					Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: attrName, VendorID: "1234", ProductID: "5678"},
					NodeName:   "node-1",
					Conditions: []metav1.Condition{{Type: string(nodeusbdevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodeusbdevicecondition.Ready), Message: "Node status"}},
				},
			}

			vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
			vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice, nodeUSBDevice).WithIndex(vmObj, vmField, vmExtractValue).WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).Build()

			res := reconciler.NewResource(
				types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
				cl,
				func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
				func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			st := state.New(cl, res)
			h := NewLifecycleHandler(cl)
			_, err := h.Handle(ctx, st)
			Expect(err).NotTo(HaveOccurred())

			template := &resourcev1.ResourceClaimTemplate{}
			err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("usb-device-cr"), Namespace: "default"}, template)
			Expect(err).NotTo(HaveOccurred())
			Expect(template.Spec.Spec.Devices.Requests).To(HaveLen(1))
			Expect(template.Spec.Spec.Devices.Requests[0].Name).To(Equal("req-usb-device-cr"))
			Expect(template.Spec.Spec.Devices.Requests[0].Exactly.Selectors).To(HaveLen(1))
			Expect(template.Spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL).NotTo(BeNil())
			Expect(template.Spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL.Expression).To(ContainSubstring(`device.attributes["virtualization-usb"].name == "` + expectedSelectorName + `"`))
		},
		Entry("uses attribute name in selector", "usb-raw-device", "usb-raw-device"),
	)

	DescribeTable("buildResourceClaimTemplateSpec selector fallback",
		func(attrName, expectedSelectorName string) {
			spec := buildResourceClaimTemplateSpec(&v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-cr", Namespace: "default"},
				Status:     v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: attrName}},
			})

			Expect(spec.Spec.Devices.Requests).To(HaveLen(1))
			Expect(spec.Spec.Devices.Requests[0].Name).To(Equal("req-usb-device-cr"))
			Expect(spec.Spec.Devices.Requests[0].Exactly.Selectors).To(HaveLen(1))
			Expect(spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL).NotTo(BeNil())
			Expect(spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL.Expression).To(ContainSubstring(`device.attributes["virtualization-usb"].name == "` + expectedSelectorName + `"`))
		},
		Entry("uses provided attribute name", "usb-raw-device", "usb-raw-device"),
		Entry("falls back to resource name when attribute name is empty", "", "usb-device-cr"),
	)

	It("should update existing ResourceClaimTemplate when selector drifts", func() {
		usbDevice := &v1alpha2.USBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-cr", Namespace: "default", UID: "usb-uid-1"},
			Status:     v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-new-name", VendorID: "0000", ProductID: "0000"}},
		}

		nodeUSBDevice := &v1alpha2.NodeUSBDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "usb-device-cr"},
			Status: v1alpha2.NodeUSBDeviceStatus{
				Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-new-name", VendorID: "1234", ProductID: "5678"},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodeusbdevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodeusbdevicecondition.Ready), Message: "Node status"}},
			},
		}

		template := &resourcev1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: ResourceClaimTemplateName("usb-device-cr"), Namespace: "default"},
			Spec: buildResourceClaimTemplateSpec(&v1alpha2.USBDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "usb-device-cr", Namespace: "default"},
				Status:     v1alpha2.USBDeviceStatus{Attributes: v1alpha2.NodeUSBDeviceAttributes{Name: "usb-old-name"}},
			}),
		}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByUSBDevice()
		vmNodeObj, vmNodeField, vmNodeExtractValue := indexer.IndexVMByNode()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(usbDevice, nodeUSBDevice, template).WithIndex(vmObj, vmField, vmExtractValue).WithIndex(vmNodeObj, vmNodeField, vmNodeExtractValue).Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: usbDevice.Name, Namespace: usbDevice.Namespace},
			cl,
			func() *v1alpha2.USBDevice { return &v1alpha2.USBDevice{} },
			func(obj *v1alpha2.USBDevice) v1alpha2.USBDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		updated := &resourcev1.ResourceClaimTemplate{}
		err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("usb-device-cr"), Namespace: "default"}, updated)
		Expect(err).NotTo(HaveOccurred())
		expr := updated.Spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL.Expression
		Expect(expr).To(ContainSubstring(`device.attributes["virtualization-usb"].name == "usb-new-name"`))
	})
})
