/*
Copyright 2025 Flant JSC

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaclcondition"
)

var _ = Describe("BoundHandler", func() {
	const macAddress = "f6:e1:74:94:12:34"

	var (
		scheme       *runtime.Scheme
		ctx          context.Context
		vmmac        *v1alpha2.VirtualMachineMACAddress
		lease        *v1alpha2.VirtualMachineMACAddressLease
		svc          *MACAddressServiceMock
		recorderMock *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())

		ctx = context.TODO()

		vmmac = &v1alpha2.VirtualMachineMACAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmmac",
				Namespace: "ns",
			},
			Spec: v1alpha2.VirtualMachineMACAddressSpec{},
		}

		lease = &v1alpha2.VirtualMachineMACAddressLease{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					annotations.LabelVirtualMachineMACAddressUID: string(vmmac.UID),
				},
				Name:       mac.AddressToLeaseName(macAddress),
				Generation: 1,
			},
		}

		svc = &MACAddressServiceMock{
			GetLeaseFunc: func(ctx context.Context, vmmac *v1alpha2.VirtualMachineMACAddress) (*v1alpha2.VirtualMachineMACAddressLease, error) {
				return nil, nil
			},
			GetAllocatedAddressesFunc: func(ctx context.Context) (mac.AllocatedMACs, error) {
				return nil, nil
			},
			AllocateNewAddressFunc: func(_ mac.AllocatedMACs) (string, error) {
				return macAddress, nil
			},
		}

		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}
	})

	Context("Lease is not created yet", func() {
		It("creates a new lease", func() {
			var leaseCreated bool
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects().
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
						_, ok := obj.(*v1alpha2.VirtualMachineMACAddressLease)
						Expect(ok).To(BeTrue())
						leaseCreated = true
						return nil
					},
				}).Build()

			h := NewBoundHandler(svc, k8sClient, recorderMock)
			res, err := h.Handle(ctx, vmmac)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmmac, metav1.ConditionFalse, vmmaccondition.VirtualMachineMACAddressLeaseNotReady, true)
			Expect(leaseCreated).To(BeTrue())
			Expect(vmmac.Status.Address).To(BeEmpty())
		})
	})

	Context("MAC address is already assigned", func() {
		BeforeEach(func() {
			vmmac.Status.Address = macAddress
		})

		It("is lost", func() {
			svc.GetLeaseFunc = func(_ context.Context, _ *v1alpha2.VirtualMachineMACAddress) (*v1alpha2.VirtualMachineMACAddressLease, error) {
				return nil, nil
			}
			h := NewBoundHandler(svc, nil, recorderMock)
			res, err := h.Handle(ctx, vmmac)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmmac, metav1.ConditionFalse, vmmaccondition.VirtualMachineMACAddressLeaseLost, true)
			Expect(vmmac.Status.Address).To(Equal(macAddress))
		})
	})

	Context("Binding", func() {
		It("has non-bound lease with ref", func() {
			svc.GetLeaseFunc = func(_ context.Context, _ *v1alpha2.VirtualMachineMACAddress) (*v1alpha2.VirtualMachineMACAddressLease, error) {
				lease.Spec.VirtualMachineMACAddressRef = &v1alpha2.VirtualMachineMACAddressLeaseMACAddressRef{
					Namespace: vmmac.Namespace,
					Name:      vmmac.Name,
				}
				return lease, nil
			}

			h := NewBoundHandler(svc, nil, recorderMock)
			res, err := h.Handle(ctx, vmmac)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmmac, metav1.ConditionFalse, vmmaccondition.VirtualMachineMACAddressLeaseNotReady, true)
			Expect(vmmac.Status.Address).To(Equal(macAddress))
		})

		It("has bound lease", func() {
			svc.GetLeaseFunc = func(_ context.Context, _ *v1alpha2.VirtualMachineMACAddress) (*v1alpha2.VirtualMachineMACAddressLease, error) {
				lease.Spec.VirtualMachineMACAddressRef = &v1alpha2.VirtualMachineMACAddressLeaseMACAddressRef{
					Namespace: vmmac.Namespace,
					Name:      vmmac.Name,
				}
				lease.Status.Conditions = append(lease.Status.Conditions, metav1.Condition{
					Type:               vmmaclcondition.BoundType.String(),
					Status:             metav1.ConditionTrue,
					Reason:             vmmaclcondition.Bound.String(),
					ObservedGeneration: lease.Generation,
				})
				return lease, nil
			}

			h := NewBoundHandler(svc, nil, recorderMock)
			res, err := h.Handle(ctx, vmmac)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmmac, metav1.ConditionTrue, vmmaccondition.Bound, false)
			Expect(vmmac.Status.Address).To(Equal(macAddress))
		})
	})
})

func ExpectCondition(vmmac *v1alpha2.VirtualMachineMACAddress, status metav1.ConditionStatus, reason vmmaccondition.BoundReason, msgExists bool) {
	ready, _ := conditions.GetCondition(vmmaccondition.BoundType, vmmac.Status.Conditions)
	Expect(ready.Status).To(Equal(status))
	Expect(ready.Reason).To(Equal(reason.String()))
	Expect(ready.ObservedGeneration).To(Equal(vmmac.Generation))

	if msgExists {
		Expect(ready.Message).ToNot(BeEmpty())
	} else {
		Expect(ready.Message).To(BeEmpty())
	}
}
