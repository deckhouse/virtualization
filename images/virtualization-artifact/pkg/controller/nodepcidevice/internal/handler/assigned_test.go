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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodepcidevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodepcidevicecondition"
)

var _ = Describe("AssignedHandler", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	DescribeTable("Handle",
		func(assignedNamespace string, namespaceExists, readyNotFound, seedPCI bool, absentFor time.Duration, expectReason string, expectPCIInAssignedNS bool) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			node := &v1alpha2.NodePCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", UID: types.UID("node-pci-uid")},
				Spec:       v1alpha2.NodePCIDeviceSpec{AssignedNamespace: assignedNamespace},
				Status: v1alpha2.NodePCIDeviceStatus{
					Attributes: v1alpha2.NodePCIDeviceAttributes{VendorID: "1234", DeviceID: "5678"},
					NodeName:   "node-1",
				},
			}
			if readyNotFound {
				node.Status.Conditions = []metav1.Condition{{
					Type:               string(nodepcidevicecondition.ReadyType),
					Status:             metav1.ConditionFalse,
					Reason:             string(nodepcidevicecondition.NotFound),
					LastTransitionTime: metav1.NewTime(time.Now().Add(-absentFor)),
				}}
			}

			objects := []client.Object{node}
			if assignedNamespace != "" && namespaceExists {
				objects = append(objects, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: assignedNamespace}})
			}
			if seedPCI {
				seedNS := assignedNamespace
				if seedNS == "" {
					seedNS = "stale-namespace"
				}
				objects = append(objects, &v1alpha2.PCIDevice{ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: seedNS}})
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithStatusSubresource(&v1alpha2.PCIDevice{}).
				WithIndex(&v1alpha2.PCIDevice{}, indexer.IndexFieldPCIDeviceByName, func(object client.Object) []string {
					pciDevice, ok := object.(*v1alpha2.PCIDevice)
					if !ok || pciDevice == nil {
						return nil
					}
					return []string{pciDevice.Name}
				}).
				Build()
			res := reconciler.NewResource(
				types.NamespacedName{Name: node.Name},
				cl,
				func() *v1alpha2.NodePCIDevice { return &v1alpha2.NodePCIDevice{} },
				func(obj *v1alpha2.NodePCIDevice) v1alpha2.NodePCIDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			h := NewAssignedHandler(cl)
			st := state.New(cl, res)
			_, err := h.Handle(ctx, st)
			Expect(err).NotTo(HaveOccurred())

			assigned := meta.FindStatusCondition(res.Changed().Status.Conditions, string(nodepcidevicecondition.AssignedType))
			Expect(assigned).NotTo(BeNil())
			Expect(assigned.Reason).To(Equal(expectReason))

			if assignedNamespace != "" {
				pci := &v1alpha2.PCIDevice{}
				err = cl.Get(ctx, types.NamespacedName{Name: "pci-device-1", Namespace: assignedNamespace}, pci)
				if expectPCIInAssignedNS {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			}
		},
		Entry("assigned namespace exists creates/keeps PCIDevice", "test-namespace", true, false, false, time.Duration(0), string(nodepcidevicecondition.Assigned), true),
		Entry("assigned namespace missing marks available", "missing-namespace", false, false, false, time.Duration(0), string(nodepcidevicecondition.Available), false),
		Entry("device recently absent on host keeps PCIDevice", "test-namespace", true, true, true, time.Minute, string(nodepcidevicecondition.Assigned), true),
		Entry("device absent on host beyond the grace period removes PCIDevice", "test-namespace", true, true, true, pciDeviceAbsenceGracePeriod+time.Minute, string(nodepcidevicecondition.InProgress), false),
		Entry("unassigned removes stale PCIDevice", "", false, false, true, time.Duration(0), string(nodepcidevicecondition.Available), false),
	)

	It("waits for a terminating PCIDevice before recreating it", func() {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		node := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", UID: types.UID("node-pci-uid")},
			Spec:       v1alpha2.NodePCIDeviceSpec{AssignedNamespace: "test-namespace"},
			Status:     v1alpha2.NodePCIDeviceStatus{NodeName: "node-1"},
		}
		now := metav1.Now()
		terminating := &v1alpha2.PCIDevice{ObjectMeta: metav1.ObjectMeta{
			Name:              "pci-device-1",
			Namespace:         "test-namespace",
			Finalizers:        []string{v1alpha2.FinalizerPCIDeviceCleanup},
			DeletionTimestamp: &now,
		}}
		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"}}, terminating).
			WithIndex(&v1alpha2.PCIDevice{}, indexer.IndexFieldPCIDeviceByName, func(object client.Object) []string {
				return []string{object.GetName()}
			}).
			Build()
		res := reconciler.NewResource(
			types.NamespacedName{Name: node.Name},
			cl,
			func() *v1alpha2.NodePCIDevice { return &v1alpha2.NodePCIDevice{} },
			func(obj *v1alpha2.NodePCIDevice) v1alpha2.NodePCIDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		result, err := NewAssignedHandler(cl).Handle(ctx, state.New(cl, res))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		assigned := meta.FindStatusCondition(res.Changed().Status.Conditions, string(nodepcidevicecondition.AssignedType))
		Expect(assigned).NotTo(BeNil())
		Expect(assigned.Reason).To(Equal(string(nodepcidevicecondition.InProgress)))
	})

	It("should ignore not found error when deleting orphaned PCIDevice", func() {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		node := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", UID: types.UID("node-pci-uid")},
			Status: v1alpha2.NodePCIDeviceStatus{
				Attributes: v1alpha2.NodePCIDeviceAttributes{VendorID: "1234", DeviceID: "5678"},
				NodeName:   "node-1",
			},
		}
		pciDevice := &v1alpha2.PCIDevice{ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "stale-namespace"}}

		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node, pciDevice).
			WithIndex(&v1alpha2.PCIDevice{}, indexer.IndexFieldPCIDeviceByName, func(object client.Object) []string {
				pciDevice, ok := object.(*v1alpha2.PCIDevice)
				if !ok || pciDevice == nil {
					return nil
				}
				return []string{pciDevice.Name}
			}).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
					if _, ok := obj.(*v1alpha2.PCIDevice); ok {
						return errors.New(`pcidevices.virtualization.deckhouse.io "pci-device-1" not found`)
					}
					return nil
				},
			}).
			Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: node.Name},
			cl,
			func() *v1alpha2.NodePCIDevice { return &v1alpha2.NodePCIDevice{} },
			func(obj *v1alpha2.NodePCIDevice) v1alpha2.NodePCIDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		h := NewAssignedHandler(cl)
		st := state.New(cl, res)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should update existing PCIDevice when attributes change", func() {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		node := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", UID: types.UID("node-pci-uid")},
			Spec:       v1alpha2.NodePCIDeviceSpec{AssignedNamespace: "test-ns"},
			Status: v1alpha2.NodePCIDeviceStatus{
				Attributes: v1alpha2.NodePCIDeviceAttributes{VendorID: "5678", DeviceID: "1234", Name: "updated-name"},
				NodeName:   "node-1",
			},
		}

		existingPCI := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", Namespace: "test-ns"},
			Status:     v1alpha2.PCIDeviceStatus{Attributes: v1alpha2.NodePCIDeviceAttributes{VendorID: "1111", DeviceID: "2222"}},
		}

		objects := []client.Object{node, existingPCI, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}}

		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objects...).
			WithStatusSubresource(&v1alpha2.PCIDevice{}).
			WithIndex(&v1alpha2.PCIDevice{}, indexer.IndexFieldPCIDeviceByName, func(object client.Object) []string {
				pciDevice, ok := object.(*v1alpha2.PCIDevice)
				if !ok || pciDevice == nil {
					return nil
				}
				return []string{pciDevice.Name}
			}).
			Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: node.Name},
			cl,
			func() *v1alpha2.NodePCIDevice { return &v1alpha2.NodePCIDevice{} },
			func(obj *v1alpha2.NodePCIDevice) v1alpha2.NodePCIDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		h := NewAssignedHandler(cl)
		st := state.New(cl, res)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		updatedPCI := &v1alpha2.PCIDevice{}
		err = cl.Get(ctx, types.NamespacedName{Name: "pci-device-1", Namespace: "test-ns"}, updatedPCI)
		Expect(err).NotTo(HaveOccurred())
		Expect(updatedPCI.Status.Attributes.VendorID).To(Equal("5678"))
		Expect(updatedPCI.Status.Attributes.DeviceID).To(Equal("1234"))
	})

	It("should skip processing when NodePCIDevice is being deleted", func() {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

		now := metav1.Now()
		node := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pci-device-1",
				UID:               types.UID("node-pci-uid"),
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerNodePCIDeviceCleanup},
			},
			Spec: v1alpha2.NodePCIDeviceSpec{AssignedNamespace: "test-ns"},
			Status: v1alpha2.NodePCIDeviceStatus{
				Attributes: v1alpha2.NodePCIDeviceAttributes{VendorID: "1234", DeviceID: "5678"},
				NodeName:   "node-1",
			},
		}

		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(node).
			Build()

		res := reconciler.NewResource(
			types.NamespacedName{Name: node.Name},
			cl,
			func() *v1alpha2.NodePCIDevice { return &v1alpha2.NodePCIDevice{} },
			func(obj *v1alpha2.NodePCIDevice) v1alpha2.NodePCIDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		h := NewAssignedHandler(cl)
		st := state.New(cl, res)
		_, err := h.Handle(ctx, st)
		Expect(err).NotTo(HaveOccurred())

		// No conditions should be set when deleting
		Expect(res.Changed().Status.Conditions).To(BeEmpty())
	})
})
