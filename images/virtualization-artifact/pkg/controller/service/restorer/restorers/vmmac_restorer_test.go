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

package restorer

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMMACTestArgs struct {
	mode v1alpha2.VMOPRestoreMode

	vmmacExists           bool
	vmmacUsedByDiffVM     bool
	staticMACUsedByDiffVM bool
	hasStaticMAC          bool

	failValidation bool
	failProcess    bool

	shouldBeDeleted bool
	shouldBeCreated bool
}

var _ = Describe("VirtualMachineMACAddressRestorer", func() {
	var (
		err error
		ctx context.Context

		uid       string
		vm        string
		name      string
		namespace string
		staticMAC string

		intercept    interceptor.Funcs
		vmmacDeleted bool
		vmmacCreated bool

		objects    []client.Object
		vmmac      v1alpha2.VirtualMachineMACAddress
		handler    *VirtualMachineMACHandler
		fakeClient client.WithWatch
	)

	BeforeEach(func() {
		ctx = context.Background()
		name = "test-vmmac"
		namespace = "default"
		staticMAC = "10.0.0.1"
		vm = "test-vm"
		uid = "0000-1111-2222-4444"

		vmmacDeleted = false
		vmmacCreated = false

		objects = []client.Object{}

		vmmac = v1alpha2.VirtualMachineMACAddress{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineMACAddress",
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineMACAddressSpec{},
			Status: v1alpha2.VirtualMachineMACAddressStatus{
				VirtualMachine: vm,
				Phase:          v1alpha2.VirtualMachineMACAddressPhaseAttached,
			},
		}

		intercept = interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if obj.GetName() == vmmac.Name {
					_, ok := obj.(*v1alpha2.VirtualMachineMACAddress)
					Expect(ok).To(BeTrue())
					vmmacDeleted = true
				}
				return nil
			},
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if obj.GetName() == vmmac.Name {
					_, ok := obj.(*v1alpha2.VirtualMachineMACAddress)
					Expect(ok).To(BeTrue())
					vmmacCreated = true
				}

				return nil
			},
		}
	})

	DescribeTable("Checking VMOP events",
		func(args VMMACTestArgs) {
			if args.hasStaticMAC {
				vmmac.Spec.Address = staticMAC
				vmmac.Status.Address = staticMAC
			}

			if args.vmmacExists {
				objects = append(objects, &vmmac)
			}

			if args.staticMACUsedByDiffVM {
				objects = append(objects, &v1alpha2.VirtualMachineMACAddress{
					ObjectMeta: metav1.ObjectMeta{Name: name + "-2", Namespace: namespace},
					Spec: v1alpha2.VirtualMachineMACAddressSpec{
						Address: staticMAC,
					},
					Status: v1alpha2.VirtualMachineMACAddressStatus{
						VirtualMachine: vm + "-2",
						Phase:          v1alpha2.VirtualMachineMACAddressPhaseAttached,
						Address:        staticMAC,
					},
				})
			}

			if args.vmmacUsedByDiffVM {
				vmmac.Status.VirtualMachine = vm + "-2"
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept, objects...)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeClient).ToNot(BeNil())

			vmmac.Status.VirtualMachine = vm
			handler = NewVirtualMachineMACAddressHandler(fakeClient, &vmmac, uid)
			Expect(handler).ToNot(BeNil())

			err = handler.ValidateRestore(ctx)
			if args.failValidation {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			err = handler.ProcessRestore(ctx)
			if args.failProcess {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(vmmacDeleted).To(Equal(args.shouldBeDeleted))
			Expect(vmmacCreated).To(Equal(args.shouldBeCreated))
		},
		Entry("vmmac exists; vmmac has auto MAC; vmmac used by different VM", VMMACTestArgs{
			mode:                  v1alpha2.VMOPRestoreModeStrict,
			vmmacExists:           true,
			hasStaticMAC:          false,
			vmmacUsedByDiffVM:     true,
			staticMACUsedByDiffVM: false,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmmac exists; vmmac has auto MAC; vmmac doesn't used by different VM", VMMACTestArgs{
			mode:                  v1alpha2.VMOPRestoreModeStrict,
			vmmacExists:           true,
			hasStaticMAC:          false,
			vmmacUsedByDiffVM:     false,
			staticMACUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmmac exists; vmmac has static MAC; vmmac used by different VM", VMMACTestArgs{
			mode:                  v1alpha2.VMOPRestoreModeStrict,
			vmmacExists:           true,
			hasStaticMAC:          true,
			vmmacUsedByDiffVM:     true,
			staticMACUsedByDiffVM: false,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmmac exists; vmmac has static MAC; static MAC used by different VM", VMMACTestArgs{
			mode:                  v1alpha2.VMOPRestoreModeStrict,
			vmmacExists:           true,
			hasStaticMAC:          true,
			vmmacUsedByDiffVM:     false,
			staticMACUsedByDiffVM: true,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmmac exists; vmmac has static MAC; vmmac doesn't used by different VM", VMMACTestArgs{
			mode:                  v1alpha2.VMOPRestoreModeStrict,
			vmmacExists:           true,
			hasStaticMAC:          true,
			vmmacUsedByDiffVM:     false,
			staticMACUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),

		Entry("vmmac doesn't exist; vmmac has auto MAC", VMMACTestArgs{
			mode:                  v1alpha2.VMOPRestoreModeStrict,
			vmmacExists:           false,
			hasStaticMAC:          false,
			vmmacUsedByDiffVM:     false,
			staticMACUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: true,
		}),
		Entry("vmmac doesn't exist; vmmac has static MAC; static MAC used by different VM", VMMACTestArgs{
			mode:                  v1alpha2.VMOPRestoreModeStrict,
			vmmacExists:           false,
			hasStaticMAC:          true,
			vmmacUsedByDiffVM:     false,
			staticMACUsedByDiffVM: true,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmmac doesn't exist; vmmac has static MAC; static MAC doesn't used by different VM", VMMACTestArgs{
			mode:                  v1alpha2.VMOPRestoreModeStrict,
			vmmacExists:           false,
			hasStaticMAC:          true,
			vmmacUsedByDiffVM:     false,
			staticMACUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: true,
		}),
	)

	Describe("Override", func() {
		var rules []v1alpha2.NameReplacement

		BeforeEach(func() {
			rules = []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachineMACAddress",
						Name: name,
					},
					To: "new-vmip-name",
				},
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept)
			Expect(err).ToNot(HaveOccurred())

			handler = NewVirtualMachineMACAddressHandler(fakeClient, &vmmac, uid)
		})

		It("should override VMMAC name", func() {
			handler.Override(rules)
			Expect(handler.vmmac.Name).To(Equal("new-vmip-name"))
		})

		It("should not override non-matching names", func() {
			nonMatchingRules := []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachineMACAddress",
						Name: "different-vmmac",
					},
					To: "should-not-apply",
				},
			}

			originalName := handler.vmmac.Name
			handler.Override(nonMatchingRules)
			Expect(handler.vmmac.Name).To(Equal(originalName))
		})
	})
})
