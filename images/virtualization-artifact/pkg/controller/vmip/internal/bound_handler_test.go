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
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmiplcondition"
)

var _ = Describe("BoundHandler", func() {
	const ipAddress = "10.10.10.10"

	var (
		scheme *runtime.Scheme
		ctx    context.Context
		vmip   *virtv2.VirtualMachineIPAddress
		lease  *virtv2.VirtualMachineIPAddressLease
		svc    *IPAddressServiceMock
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(virtv2.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())

		ctx = context.TODO()

		vmip = &virtv2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vmip",
				Namespace: "ns",
			},
			Spec: virtv2.VirtualMachineIPAddressSpec{
				Type: virtv2.VirtualMachineIPAddressTypeAuto,
			},
		}

		lease = &virtv2.VirtualMachineIPAddressLease{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					annotations.LabelVirtualMachineIPAddressUID: string(vmip.UID),
				},
				Name:       ip.IPToLeaseName(ipAddress),
				Generation: 1,
			},
		}

		svc = &IPAddressServiceMock{
			GetLeaseFunc: func(ctx context.Context, vmip *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
				return nil, nil
			},
			GetAllocatedIPsFunc: func(ctx context.Context) (ip.AllocatedIPs, error) {
				return nil, nil
			},
			AllocateNewIPFunc: func(_ ip.AllocatedIPs) (string, error) {
				return ipAddress, nil
			},
			IsInsideOfRangeFunc: func(_ string) error {
				return nil
			},
		}
	})

	Context("Lease is not created yet", func() {
		It("creates a new lease", func() {
			var leaseCreated bool
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects().
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
						_, ok := obj.(*virtv2.VirtualMachineIPAddressLease)
						Expect(ok).To(BeTrue())
						leaseCreated = true
						return nil
					},
				}).Build()

			h := NewBoundHandler(svc, k8sClient)
			res, err := h.Handle(ctx, vmip)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmip, metav1.ConditionFalse, vmipcondition.VirtualMachineIPAddressLeaseNotReady, true)
			Expect(leaseCreated).To(BeTrue())
			Expect(vmip.Status.Address).To(BeEmpty())
		})
	})

	Context("IP address is already assigned", func() {
		BeforeEach(func() {
			vmip.Status.Address = ipAddress
		})

		It("takes existing released lease", func() {
			var leaseUpdated bool
			svc.GetLeaseFunc = func(_ context.Context, _ *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
				lease.Spec.VirtualMachineIPAddressRef = nil
				return lease, nil
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects().
				WithInterceptorFuncs(interceptor.Funcs{
					Update: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.UpdateOption) error {
						updatedLease, ok := obj.(*virtv2.VirtualMachineIPAddressLease)
						Expect(ok).To(BeTrue())
						Expect(updatedLease.Spec.VirtualMachineIPAddressRef).NotTo(BeNil())
						Expect(updatedLease.Spec.VirtualMachineIPAddressRef.Name).To(Equal(vmip.Name))
						Expect(updatedLease.Spec.VirtualMachineIPAddressRef.Namespace).To(Equal(vmip.Namespace))
						Expect(updatedLease.Labels[annotations.LabelVirtualMachineIPAddressUID]).To(Equal(string(vmip.UID)))
						leaseUpdated = true
						return nil
					},
				}).Build()

			h := NewBoundHandler(svc, k8sClient)
			res, err := h.Handle(ctx, vmip)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmip, metav1.ConditionFalse, vmipcondition.VirtualMachineIPAddressLeaseNotReady, true)
			Expect(vmip.Status.Address).To(Equal(ipAddress))
			Expect(leaseUpdated).To(BeTrue())
		})

		It("cannot take existing lease: it's bound to another vmip", func() {
			svc.GetLeaseFunc = func(_ context.Context, _ *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
				lease.Spec.VirtualMachineIPAddressRef = &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
					Namespace: vmip.Namespace,
					Name:      "another-vmip",
				}
				return lease, nil
			}
			h := NewBoundHandler(svc, nil)
			res, err := h.Handle(ctx, vmip)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmip, metav1.ConditionFalse, vmipcondition.VirtualMachineIPAddressLeaseNotReady, true)
			Expect(vmip.Status.Address).To(Equal(ipAddress))
		})

		It("cannot take existing lease: it belongs to different namespace", func() {
			svc.GetLeaseFunc = func(_ context.Context, _ *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
				lease.Spec.VirtualMachineIPAddressRef = &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
					Namespace: vmip.Namespace + "-different",
				}
				return lease, nil
			}
			h := NewBoundHandler(svc, nil)
			res, err := h.Handle(ctx, vmip)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmip, metav1.ConditionFalse, vmipcondition.VirtualMachineIPAddressLeaseNotReady, true)
			Expect(vmip.Status.Address).To(Equal(ipAddress))
		})

		It("is lost", func() {
			svc.GetLeaseFunc = func(_ context.Context, _ *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
				return nil, nil
			}
			h := NewBoundHandler(svc, nil)
			res, err := h.Handle(ctx, vmip)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmip, metav1.ConditionFalse, vmipcondition.VirtualMachineIPAddressLeaseLost, true)
			Expect(vmip.Status.Address).To(Equal(ipAddress))
		})
	})

	Context("Binding", func() {
		It("has non-bound lease with ref", func() {
			svc.GetLeaseFunc = func(_ context.Context, _ *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
				lease.Spec.VirtualMachineIPAddressRef = &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
					Namespace: vmip.Namespace,
					Name:      vmip.Name,
				}
				return lease, nil
			}

			h := NewBoundHandler(svc, nil)
			res, err := h.Handle(ctx, vmip)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmip, metav1.ConditionFalse, vmipcondition.VirtualMachineIPAddressLeaseNotReady, true)
			Expect(vmip.Status.Address).To(Equal(ipAddress))
		})

		It("has bound lease", func() {
			svc.GetLeaseFunc = func(_ context.Context, _ *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
				lease.Spec.VirtualMachineIPAddressRef = &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
					Namespace: vmip.Namespace,
					Name:      vmip.Name,
				}
				lease.Status.Conditions = append(lease.Status.Conditions, metav1.Condition{
					Type:               vmiplcondition.BoundType.String(),
					Status:             metav1.ConditionTrue,
					Reason:             vmiplcondition.Bound.String(),
					ObservedGeneration: lease.Generation,
				})
				return lease, nil
			}

			h := NewBoundHandler(svc, nil)
			res, err := h.Handle(ctx, vmip)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.IsZero()).To(BeTrue())

			ExpectCondition(vmip, metav1.ConditionTrue, vmipcondition.Bound, false)
			Expect(vmip.Status.Address).To(Equal(ipAddress))
		})
	})
})

func ExpectCondition(vmip *virtv2.VirtualMachineIPAddress, status metav1.ConditionStatus, reason vmipcondition.BoundReason, msgExists bool) {
	ready, _ := conditions.GetCondition(vmipcondition.BoundType, vmip.Status.Conditions)
	Expect(ready.Status).To(Equal(status))
	Expect(ready.Reason).To(Equal(reason.String()))
	Expect(ready.ObservedGeneration).To(Equal(vmip.Generation))

	if msgExists {
		Expect(ready.Message).ToNot(BeEmpty())
	} else {
		Expect(ready.Message).To(BeEmpty())
	}
}
