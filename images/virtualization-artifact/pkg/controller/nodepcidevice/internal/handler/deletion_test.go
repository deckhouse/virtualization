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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/nodepcidevice/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodepcidevicecondition"
)

var _ = Describe("DeletionHandler", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = logger.ToContext(context.TODO(), slog.Default())
	})

	DescribeTable("Handle",
		func(deleting, autoDelete, withOwnedPCI, withFinalizer bool, assignedNamespace, pciNamespace string, expectFinalizerPresent, expectOwnedPCIDeleted, expectNodeDeleted, expectStopChain bool) {
			scheme := apiruntime.NewScheme()
			Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

			node := &v1alpha2.NodePCIDevice{
				ObjectMeta: metav1.ObjectMeta{Name: "pci-device-1", UID: "node-pci-uid"},
				Spec:       v1alpha2.NodePCIDeviceSpec{AssignedNamespace: assignedNamespace},
			}
			if autoDelete {
				node.Status.Conditions = []metav1.Condition{{
					Type:   string(nodepcidevicecondition.ReadyType),
					Status: metav1.ConditionFalse,
					Reason: string(nodepcidevicecondition.NotFound),
				}}
			}
			if withFinalizer {
				node.Finalizers = []string{v1alpha2.FinalizerNodePCIDeviceCleanup}
			}
			if deleting {
				now := metav1.Now()
				node.DeletionTimestamp = &now
			}

			objects := []client.Object{node}
			if withOwnedPCI {
				objects = append(objects, &v1alpha2.PCIDevice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pci-device-1",
						Namespace: pciNamespace,
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: v1alpha2.SchemeGroupVersion.String(),
							Kind:       v1alpha2.NodePCIDeviceKind,
							Name:       node.Name,
							UID:        node.UID,
							Controller: ptr.To(true),
						}},
					},
				})
			}

			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			res := reconciler.NewResource(
				types.NamespacedName{Name: node.Name},
				cl,
				func() *v1alpha2.NodePCIDevice { return &v1alpha2.NodePCIDevice{} },
				func(obj *v1alpha2.NodePCIDevice) v1alpha2.NodePCIDeviceStatus { return obj.Status },
			)
			Expect(res.Fetch(ctx)).To(Succeed())

			h := NewDeletionHandler(cl)
			st := state.New(cl, res)
			_, err := h.Handle(ctx, st)
			if expectStopChain {
				Expect(err).To(MatchError(reconciler.ErrStopHandlerChain))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			if expectFinalizerPresent {
				Expect(res.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerNodePCIDeviceCleanup))
			} else {
				Expect(res.Changed().GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerNodePCIDeviceCleanup))
			}

			if withOwnedPCI {
				pci := &v1alpha2.PCIDevice{}
				err = cl.Get(ctx, types.NamespacedName{Name: "pci-device-1", Namespace: pciNamespace}, pci)
				if expectOwnedPCIDeleted {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			}

			deletedNode := &v1alpha2.NodePCIDevice{}
			err = cl.Get(ctx, types.NamespacedName{Name: node.Name}, deletedNode)
			if expectNodeDeleted {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("not deleting adds finalizer", false, false, false, false, "", "", true, false, false, false),
		Entry("auto-delete first adds finalizer without deleting node object", false, true, true, false, "", "test-namespace", true, false, false, false),
		Entry("auto-delete marks node object for deletion when finalizer is already present", false, true, true, true, "", "test-namespace", true, false, false, true),
		Entry("assigned not found device is not auto-deleted", false, true, false, true, "test-namespace", "", true, false, false, false),
		Entry("deleting removes finalizer and owned PCI", true, false, true, true, "", "test-namespace", false, true, false, false),
		Entry("deleting removes finalizer even without owned PCI", true, false, false, true, "", "", false, false, false, false),
		Entry("deleting removes owned PCI in different namespace", true, false, true, true, "", "previous-namespace", false, true, false, false),
	)

	It("keeps the finalizer while an owned PCIDevice is still terminating", func() {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

		now := metav1.Now()
		node := &v1alpha2.NodePCIDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pci-device-1",
				UID:               "node-pci-uid",
				Finalizers:        []string{v1alpha2.FinalizerNodePCIDeviceCleanup},
				DeletionTimestamp: &now,
			},
		}
		pci := &v1alpha2.PCIDevice{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "pci-device-1",
				Namespace:  "test-namespace",
				Finalizers: []string{v1alpha2.FinalizerPCIDeviceCleanup},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.NodePCIDeviceKind,
					Name:       node.Name,
					UID:        node.UID,
					Controller: ptr.To(true),
				}},
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, pci).Build()
		res := reconciler.NewResource(
			types.NamespacedName{Name: node.Name},
			cl,
			func() *v1alpha2.NodePCIDevice { return &v1alpha2.NodePCIDevice{} },
			func(obj *v1alpha2.NodePCIDevice) v1alpha2.NodePCIDeviceStatus { return obj.Status },
		)
		Expect(res.Fetch(ctx)).To(Succeed())

		result, err := NewDeletionHandler(cl).Handle(ctx, state.New(cl, res))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(res.Changed().GetFinalizers()).To(ContainElement(v1alpha2.FinalizerNodePCIDeviceCleanup))

		terminating := &v1alpha2.PCIDevice{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: pci.Name, Namespace: pci.Namespace}, terminating)).To(Succeed())
		Expect(terminating.GetDeletionTimestamp().IsZero()).To(BeFalse())
	})
})
