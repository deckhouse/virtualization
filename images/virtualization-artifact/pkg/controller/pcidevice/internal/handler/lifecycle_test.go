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
	"errors"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/pcidevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodepcidevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/pcidevicecondition"
)

var _ = Describe("LifecycleHandler", func() {
	var ctx context.Context
	var scheme *apiruntime.Scheme

	newResource := func(cl client.Client, pciDevice *v1alpha2.PCIDevice) *reconciler.Resource[*v1alpha2.PCIDevice, v1alpha2.PCIDeviceStatus] {
		res := reconciler.NewResource(
			types.NamespacedName{Name: pciDevice.Name, Namespace: pciDevice.Namespace},
			cl,
			func() *v1alpha2.PCIDevice { return &v1alpha2.PCIDevice{} },
			func(obj *v1alpha2.PCIDevice) v1alpha2.PCIDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())
		return res
	}

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
		scheme = apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(resourcev1.AddToScheme(scheme)).To(Succeed())
	})

	DescribeTable("Handle",
		func(hasNode bool, nodeConditionReason string, withVM, withMultipleVMs bool, expectReady metav1.ConditionStatus, expectReadyReason, expectAttachedReason string) {
			pciDevice := &v1alpha2.PCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "default", UID: "pci-uid-1"},
				Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-device-1", VendorID: "0000", DeviceID: "0000"}},
			}

			objects := []client.Object{pciDevice}
			if hasNode {
				objects = append(objects, &v1alpha2.NodePCIDevice{
					ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1"},
					Status: v1alpha2.NodePCIDeviceStatus{
						Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-device-1", VendorID: "1234", DeviceID: "5678"},
						NodeName:   "node-1",
						Conditions: []metav1.Condition{{Type: string(nodepcidevicecondition.ReadyType), Reason: nodeConditionReason, Message: "Node status"}},
					},
				})
			}

			if withVM {
				vm := &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "vm-1", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{PCIDevices: []v1alpha2.PCIDeviceSpecRef{{Name: "pci-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{PCIDevices: []v1alpha2.PCIDeviceStatusRef{{Name: "pci-device-1", Attached: true}}},
				}
				objects = append(objects, vm)
			}

			if withMultipleVMs {
				objects = append(objects, &v1alpha2.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{Name: "vm-2", Namespace: "default"},
					Spec:       v1alpha2.VirtualMachineSpec{PCIDevices: []v1alpha2.PCIDeviceSpecRef{{Name: "pci-device-1"}}},
					Status:     v1alpha2.VirtualMachineStatus{PCIDevices: []v1alpha2.PCIDeviceStatusRef{{Name: "pci-device-1", Attached: true}}},
				})
			}

			vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithIndex(vmObj, vmField, vmExtractValue).Build()

			res := newResource(cl, pciDevice)

			st := state.New(cl, res)
			h := NewLifecycleHandler(cl)
			_, err := h.Handle(ctx, st)
			Expect(err).NotTo(HaveOccurred())

			ready := meta.FindStatusCondition(res.Changed().Status.Conditions, string(pcidevicecondition.ReadyType))
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(expectReady))
			Expect(ready.Reason).To(Equal(expectReadyReason))

			attached := meta.FindStatusCondition(res.Changed().Status.Conditions, string(pcidevicecondition.AttachedType))
			Expect(attached).NotTo(BeNil())
			Expect(attached.Reason).To(Equal(expectAttachedReason))

			template := &resourcev1.ResourceClaimTemplate{}
			err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("pci-device-1"), Namespace: "default"}, template)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("node ready and not attached", true, string(nodepcidevicecondition.Ready), false, false, metav1.ConditionTrue, string(pcidevicecondition.Ready), string(pcidevicecondition.Available)),
		Entry("node ready and attached to one VM", true, string(nodepcidevicecondition.Ready), true, false, metav1.ConditionTrue, string(pcidevicecondition.Ready), string(pcidevicecondition.AttachedToVirtualMachine)),
		Entry("node ready and attached to multiple VMs", true, string(nodepcidevicecondition.Ready), true, true, metav1.ConditionTrue, string(pcidevicecondition.Ready), string(pcidevicecondition.AttachedToVirtualMachine)),
		Entry("node not ready", true, string(nodepcidevicecondition.NotReady), false, false, metav1.ConditionFalse, string(pcidevicecondition.NotReady), string(pcidevicecondition.Available)),
		Entry("node not found", true, string(nodepcidevicecondition.NotFound), false, false, metav1.ConditionFalse, string(pcidevicecondition.NotFound), string(pcidevicecondition.Available)),
		Entry("node missing", false, "", false, false, metav1.ConditionFalse, string(pcidevicecondition.NotFound), string(pcidevicecondition.Available)),
	)

	It("should set AttachedToVirtualMachine when VM references device even if not attached yet", func() {
		pciDevice := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "default", UID: "pci-uid-1"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-device-1", VendorID: "1234", DeviceID: "5678"}},
		}

		nodePCIDevice := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1"},
			Status: v1alpha2.NodePCIDeviceStatus{
				Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-device-1", VendorID: "1234", DeviceID: "5678"},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodepcidevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodepcidevicecondition.Ready), Message: "Node status"}},
			},
		}

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-1", Namespace: "default"},
			Spec:       v1alpha2.VirtualMachineSpec{PCIDevices: []v1alpha2.PCIDeviceSpecRef{{Name: "pci-device-1"}}},
			Status:     v1alpha2.VirtualMachineStatus{PCIDevices: []v1alpha2.PCIDeviceStatusRef{{Name: "pci-device-1", Attached: false}}},
		}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pciDevice, nodePCIDevice, vm).WithIndex(vmObj, vmField, vmExtractValue).Build()

		res := newResource(cl, pciDevice)

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		attached := meta.FindStatusCondition(res.Changed().Status.Conditions, string(pcidevicecondition.AttachedType))
		Expect(attached).NotTo(BeNil())
		Expect(attached.Reason).To(Equal(string(pcidevicecondition.AttachedToVirtualMachine)))
		Expect(attached.Status).To(Equal(metav1.ConditionTrue))
		Expect(attached.Message).To(ContainSubstring("attached to VirtualMachine default/vm-1"))
	})

	It("should not attach device from another namespace with the same name", func() {
		pciDevice := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "default", UID: "pci-uid-1"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-device-1", VendorID: "1234", DeviceID: "5678"}},
		}

		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-1", Namespace: "other"},
			Spec:       v1alpha2.VirtualMachineSpec{PCIDevices: []v1alpha2.PCIDeviceSpecRef{{Name: "pci-device-1"}}},
		}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pciDevice, vm).WithIndex(vmObj, vmField, vmExtractValue).Build()

		res := newResource(cl, pciDevice)

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		attached := meta.FindStatusCondition(res.Changed().Status.Conditions, string(pcidevicecondition.AttachedType))
		Expect(attached).NotTo(BeNil())
		Expect(attached.Reason).To(Equal(string(pcidevicecondition.Available)))
		Expect(attached.Status).To(Equal(metav1.ConditionFalse))
	})

	It("should skip ResourceClaimTemplate when attribute name is empty", func() {
		pciDevice := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "default", UID: "pci-uid-1"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "", VendorID: "0000", DeviceID: "0000"}},
		}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pciDevice).WithIndex(vmObj, vmField, vmExtractValue).Build()

		res := newResource(cl, pciDevice)

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		template := &resourcev1.ResourceClaimTemplate{}
		err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("pci-device-1"), Namespace: "default"}, template)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	DescribeTable("ResourceClaimTemplate request and selector names",
		func(attrName, expectedSelectorName string) {
			pciDevice := &v1alpha2.PCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr", Namespace: "default", UID: "pci-uid-1"},
				Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: attrName, VendorID: "0000", DeviceID: "0000"}},
			}

			nodePCIDevice := &v1alpha2.NodePCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr"},
				Status: v1alpha2.NodePCIDeviceStatus{
					Attributes: v1alpha2.NodePCIDeviceAttributes{Name: attrName, VendorID: "1234", DeviceID: "5678"},
					NodeName:   "node-1",
					Conditions: []metav1.Condition{{Type: string(nodepcidevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodepcidevicecondition.Ready), Message: "Node status"}},
				},
			}

			vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pciDevice, nodePCIDevice).WithIndex(vmObj, vmField, vmExtractValue).Build()

			res := newResource(cl, pciDevice)

			st := state.New(cl, res)
			h := NewLifecycleHandler(cl)
			_, err := h.Handle(ctx, st)
			Expect(err).NotTo(HaveOccurred())

			template := &resourcev1.ResourceClaimTemplate{}
			err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("pci-device-cr"), Namespace: "default"}, template)
			Expect(err).NotTo(HaveOccurred())
			Expect(template.Spec.Spec.Devices.Requests).To(HaveLen(1))
			Expect(template.Spec.Spec.Devices.Requests[0].Name).To(Equal("req-pci-device-cr"))
			Expect(template.Spec.Spec.Devices.Requests[0].Exactly.Selectors).To(HaveLen(1))
			Expect(template.Spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL).NotTo(BeNil())
			Expect(template.Spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL.Expression).To(ContainSubstring(`device.attributes["virtualization-pci"].name == "` + expectedSelectorName + `"`))
		},
		Entry("uses attribute name in selector", "pci-raw-device", "pci-raw-device"),
	)

	DescribeTable("buildResourceClaimTemplateSpec selector fallback",
		func(attrName, expectedSelectorName string) {
			spec := buildResourceClaimTemplateSpec(&v1alpha2.PCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr", Namespace: "default"},
				Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: attrName}},
			})

			Expect(spec.Spec.Devices.Requests).To(HaveLen(1))
			Expect(spec.Spec.Devices.Requests[0].Name).To(Equal("req-pci-device-cr"))
			Expect(spec.Spec.Devices.Requests[0].Exactly.Selectors).To(HaveLen(1))
			Expect(spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL).NotTo(BeNil())
			Expect(spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL.Expression).To(ContainSubstring(`device.attributes["virtualization-pci"].name == "` + expectedSelectorName + `"`))
		},
		Entry("uses provided attribute name", "pci-raw-device", "pci-raw-device"),
		Entry("falls back to resource name when attribute name is empty", "", "pci-device-cr"),
	)

	It("should ignore non-status already existing ResourceClaimTemplate on stale create", func() {
		pciDevice := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr", Namespace: "default", UID: "pci-uid-1"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-new-name", VendorID: "1234", DeviceID: "5678"}},
		}

		nodePCIDevice := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr"},
			Status: v1alpha2.NodePCIDeviceStatus{
				Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-new-name", VendorID: "1234", DeviceID: "5678"},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodepcidevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodepcidevicecondition.Ready), Message: "Node status"}},
			},
		}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pciDevice, nodePCIDevice).WithIndex(vmObj, vmField, vmExtractValue).WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if _, ok := obj.(*resourcev1.ResourceClaimTemplate); ok {
					return errors.New(`resourceclaimtemplates.resource.k8s.io "pci-device-cr-template" already exists`)
				}
				return nil
			},
		}).Build()

		res := newResource(cl, pciDevice)

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should update existing ResourceClaimTemplate when selector drifts", func() {
		pciDevice := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr", Namespace: "default", UID: "pci-uid-1"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-new-name", VendorID: "0000", DeviceID: "0000"}},
		}

		nodePCIDevice := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr"},
			Status: v1alpha2.NodePCIDeviceStatus{
				Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-new-name", VendorID: "1234", DeviceID: "5678"},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodepcidevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodepcidevicecondition.Ready), Message: "Node status"}},
			},
		}

		template := &resourcev1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: ResourceClaimTemplateName("pci-device-cr"), Namespace: "default"},
			Spec: buildResourceClaimTemplateSpec(&v1alpha2.PCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr", Namespace: "default"},
				Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-old-name"}},
			}),
		}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pciDevice, nodePCIDevice, template).WithIndex(vmObj, vmField, vmExtractValue).Build()

		res := newResource(cl, pciDevice)

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		updated := &resourcev1.ResourceClaimTemplate{}
		err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("pci-device-cr"), Namespace: "default"}, updated)
		Expect(err).NotTo(HaveOccurred())
		expr := updated.Spec.Spec.Devices.Requests[0].Exactly.Selectors[0].CEL.Expression
		Expect(expr).To(ContainSubstring(`device.attributes["virtualization-pci"].name == "pci-new-name"`))
		Expect(updated.Annotations).To(HaveKeyWithValue(annotations.AnnPCIClaimSpecHash, claimSpecHash(updated.Spec)))
	})

	It("should not recreate ResourceClaimTemplate without hash annotation when spec matches", func() {
		pciDevice := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr", Namespace: "default", UID: "pci-uid-1"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-name", VendorID: "1234", DeviceID: "5678"}},
		}

		nodePCIDevice := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr"},
			Status: v1alpha2.NodePCIDeviceStatus{
				Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-name", VendorID: "1234", DeviceID: "5678"},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodepcidevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodepcidevicecondition.Ready), Message: "Node status"}},
			},
		}

		template := &resourcev1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ResourceClaimTemplateName("pci-device-cr"),
				Namespace: "default",
				Labels:    map[string]string{"keep": "me"},
			},
			Spec: buildResourceClaimTemplateSpec(pciDevice),
		}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pciDevice, nodePCIDevice, template).WithIndex(vmObj, vmField, vmExtractValue).Build()

		res := newResource(cl, pciDevice)

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		stored := &resourcev1.ResourceClaimTemplate{}
		err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("pci-device-cr"), Namespace: "default"}, stored)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Labels).To(HaveKeyWithValue("keep", "me"))
		Expect(stored.Annotations).NotTo(HaveKey(annotations.AnnPCIClaimSpecHash))
	})

	It("should not recreate ResourceClaimTemplate when spec hash matches", func() {
		pciDevice := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr", Namespace: "default", UID: "pci-uid-1"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-name", VendorID: "1234", DeviceID: "5678"}},
		}

		nodePCIDevice := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr"},
			Status: v1alpha2.NodePCIDeviceStatus{
				Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-name", VendorID: "1234", DeviceID: "5678"},
				NodeName:   "node-1",
				Conditions: []metav1.Condition{{Type: string(nodepcidevicecondition.ReadyType), Status: metav1.ConditionTrue, Reason: string(nodepcidevicecondition.Ready), Message: "Node status"}},
			},
		}

		template := buildResourceClaimTemplate(pciDevice, ResourceClaimTemplateName("pci-device-cr"), buildResourceClaimTemplateSpec(pciDevice))
		template.Labels = map[string]string{"keep": "me"}

		vmObj, vmField, vmExtractValue := indexer.IndexVMByPCIDevice()
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pciDevice, nodePCIDevice, template).WithIndex(vmObj, vmField, vmExtractValue).Build()

		res := newResource(cl, pciDevice)

		st := state.New(cl, res)
		h := NewLifecycleHandler(cl)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		stored := &resourcev1.ResourceClaimTemplate{}
		err = cl.Get(ctx, types.NamespacedName{Name: ResourceClaimTemplateName("pci-device-cr"), Namespace: "default"}, stored)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Labels).To(HaveKeyWithValue("keep", "me"))
	})

	It("should not set user and group annotations on ResourceClaimTemplate spec", func() {
		spec := buildResourceClaimTemplateSpec(&v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-cr", Namespace: "default"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{Name: "pci-name"}},
		})

		Expect(spec.ObjectMeta.Annotations).To(BeEmpty())
	})
})
