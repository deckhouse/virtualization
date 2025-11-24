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

type VMIPTestArgs struct {
	mode     v1alpha2.SnapshotOperationMode
	vmipType v1alpha2.VirtualMachineIPAddressType

	vmipExists           bool
	vmipUsedByDiffVM     bool
	staticIPUsedByDiffVM bool

	failValidation bool
	failProcess    bool

	shouldBeDeleted bool
	shouldBeCreated bool
}

var _ = Describe("VirtualMachineIPAddressRestorer", func() {
	var (
		err error
		ctx context.Context

		uid       string
		vm        string
		name      string
		namespace string
		staticIP  string

		intercept   interceptor.Funcs
		vmipDeleted bool
		vmipCreated bool

		objects    []client.Object
		vmip       v1alpha2.VirtualMachineIPAddress
		handler    *VirtualMachineIPHandler
		fakeClient client.WithWatch
	)

	BeforeEach(func() {
		ctx = context.Background()
		name = "test-vmip"
		namespace = "default"
		staticIP = "10.0.0.1"
		vm = "test-vm"
		uid = "0000-1111-2222-4444"

		vmipDeleted = false
		vmipCreated = false

		objects = []client.Object{}

		vmip = v1alpha2.VirtualMachineIPAddress{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineIPAddress",
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineIPAddressSpec{},
			Status: v1alpha2.VirtualMachineIPAddressStatus{
				VirtualMachine: vm,
				Phase:          v1alpha2.VirtualMachineIPAddressPhaseAttached,
			},
		}

		intercept = interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if obj.GetName() == vmip.Name {
					_, ok := obj.(*v1alpha2.VirtualMachineIPAddress)
					Expect(ok).To(BeTrue())
					vmipDeleted = true
				}
				return nil
			},
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if obj.GetName() == vmip.Name {
					_, ok := obj.(*v1alpha2.VirtualMachineIPAddress)
					Expect(ok).To(BeTrue())
					vmipCreated = true
				}

				return nil
			},
		}
	})

	DescribeTable("Checking VMOP events",
		func(args VMIPTestArgs) {
			switch args.vmipType {
			case v1alpha2.VirtualMachineIPAddressTypeAuto:
				vmip.Spec.Type = v1alpha2.VirtualMachineIPAddressTypeAuto
			case v1alpha2.VirtualMachineIPAddressTypeStatic:
				vmip.Spec.Type = v1alpha2.VirtualMachineIPAddressTypeStatic
				vmip.Spec.StaticIP = staticIP
				vmip.Status.Address = staticIP
			}

			if args.vmipExists {
				objects = append(objects, &vmip)
			}

			if args.staticIPUsedByDiffVM {
				objects = append(objects, &v1alpha2.VirtualMachineIPAddress{
					ObjectMeta: metav1.ObjectMeta{Name: name + "-2", Namespace: namespace},
					Spec: v1alpha2.VirtualMachineIPAddressSpec{
						StaticIP: staticIP,
					},
					Status: v1alpha2.VirtualMachineIPAddressStatus{
						VirtualMachine: vm + "-2",
						Phase:          v1alpha2.VirtualMachineIPAddressPhaseAttached,
						Address:        staticIP,
					},
				})
			}

			if args.vmipUsedByDiffVM {
				vmip.Status.VirtualMachine = vm + "-2"
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept, objects...)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeClient).ToNot(BeNil())

			vmip.Status.VirtualMachine = vm
			handler = NewVirtualMachineIPAddressHandler(fakeClient, &vmip, uid)
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

			Expect(vmipDeleted).To(Equal(args.shouldBeDeleted))
			Expect(vmipCreated).To(Equal(args.shouldBeCreated))
		},
		Entry("vmip exists; vmip has Auto type; vmip used by different VM", VMIPTestArgs{
			mode:                 v1alpha2.SnapshotOperationModeStrict,
			vmipExists:           true,
			vmipType:             v1alpha2.VirtualMachineIPAddressTypeAuto,
			vmipUsedByDiffVM:     true,
			staticIPUsedByDiffVM: false,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmip exists; vmip has Auto type; vmip doesn't used by different VM", VMIPTestArgs{
			mode:                 v1alpha2.SnapshotOperationModeStrict,
			vmipExists:           true,
			vmipType:             v1alpha2.VirtualMachineIPAddressTypeAuto,
			vmipUsedByDiffVM:     false,
			staticIPUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmip exists; vmip has StaticIP type; vmip used by different VM", VMIPTestArgs{
			mode:                 v1alpha2.SnapshotOperationModeStrict,
			vmipExists:           true,
			vmipType:             v1alpha2.VirtualMachineIPAddressTypeStatic,
			vmipUsedByDiffVM:     true,
			staticIPUsedByDiffVM: false,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmip exists; vmip has StaticIP type; staticIP used by different VM", VMIPTestArgs{
			mode:                 v1alpha2.SnapshotOperationModeStrict,
			vmipExists:           true,
			vmipType:             v1alpha2.VirtualMachineIPAddressTypeStatic,
			vmipUsedByDiffVM:     false,
			staticIPUsedByDiffVM: true,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmip exists; vmip has StaticIP type; vmip doesn't used by different VM", VMIPTestArgs{
			mode:                 v1alpha2.SnapshotOperationModeStrict,
			vmipExists:           true,
			vmipType:             v1alpha2.VirtualMachineIPAddressTypeStatic,
			vmipUsedByDiffVM:     false,
			staticIPUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),

		Entry("vmip doesn't exist; vmip has Auto type", VMIPTestArgs{
			mode:                 v1alpha2.SnapshotOperationModeStrict,
			vmipExists:           false,
			vmipType:             v1alpha2.VirtualMachineIPAddressTypeAuto,
			vmipUsedByDiffVM:     false,
			staticIPUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: true,
		}),
		Entry("vmip doesn't exist; vmip has StaticIP type; staticIP used by different VM", VMIPTestArgs{
			mode:                 v1alpha2.SnapshotOperationModeStrict,
			vmipExists:           false,
			vmipType:             v1alpha2.VirtualMachineIPAddressTypeStatic,
			vmipUsedByDiffVM:     false,
			staticIPUsedByDiffVM: true,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("vmip doesn't exist; vmip has StaticIP type; staticIP doesn't used by different VM", VMIPTestArgs{
			mode:                 v1alpha2.SnapshotOperationModeStrict,
			vmipExists:           false,
			vmipType:             v1alpha2.VirtualMachineIPAddressTypeStatic,
			vmipUsedByDiffVM:     false,
			staticIPUsedByDiffVM: false,

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
						Kind: "VirtualMachineIPAddress",
						Name: name,
					},
					To: "new-vmip-name",
				},
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept)
			Expect(err).ToNot(HaveOccurred())

			handler = NewVirtualMachineIPAddressHandler(fakeClient, &vmip, uid)
		})

		It("should override VMIP name", func() {
			handler.Override(rules)
			Expect(handler.vmip.Name).To(Equal("new-vmip-name"))
		})

		It("should not override non-matching names", func() {
			nonMatchingRules := []v1alpha2.NameReplacement{
				{
					From: v1alpha2.NameReplacementFrom{
						Kind: "VirtualMachineIPAddress",
						Name: "different-vmip",
					},
					To: "should-not-apply",
				},
			}

			originalName := handler.vmip.Name
			handler.Override(nonMatchingRules)
			Expect(handler.vmip.Name).To(Equal(originalName))
		})
	})
})
