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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

var _ = Describe("AttachedHandler", func() {
	var vmip *v1alpha2.VirtualMachineIPAddress

	BeforeEach(func() {
		vmip = &v1alpha2.VirtualMachineIPAddress{ObjectMeta: metav1.ObjectMeta{Name: "vmip", Namespace: "ns"}}
	})

	handle := func(vm *v1alpha2.VirtualMachine) metav1.ConditionStatus {
		cl, err := testutil.NewFakeClientWithObjects(vm, vmip)
		Expect(err).NotTo(HaveOccurred())
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(_ client.Object, _, _, _ string) {},
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
		}
		_, err = NewAttachedHandler(recorder, cl).Handle(context.Background(), vmip)
		Expect(err).NotTo(HaveOccurred())
		cond, _ := conditions.GetCondition(vmipcondition.AttachedType, vmip.Status.Conditions)
		return cond.Status
	}

	DescribeTable("a machine naming the vmip in its spec",
		func(networks []v1alpha2.NetworksSpec, statusNetworks []v1alpha2.NetworksStatus, attached bool) {
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
				Spec:       v1alpha2.VirtualMachineSpec{VirtualMachineIPAddress: vmip.Name, Networks: networks},
				Status:     v1alpha2.VirtualMachineStatus{Networks: statusNetworks},
			}

			if attached {
				Expect(handle(vm)).To(Equal(metav1.ConditionTrue))
				Expect(vmip.Status.VirtualMachine).To(Equal(vm.Name))
			} else {
				Expect(handle(vm)).To(Equal(metav1.ConditionFalse))
				Expect(vmip.Status.VirtualMachine).To(BeEmpty())
			}
		},
		Entry("with the Main network in its status is attached", nil, []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeMain}}, true),
		Entry("listing no network, with no Main network in its status, is not attached", nil, nil, false),
		Entry("listing the Main network, with no Main network in its status, is not attached", []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}, nil, false),
	)
})
