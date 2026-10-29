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

package vm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

var _ = Describe("migration receiver webhook", func() {
	DescribeTable("isPeerUser",
		func(name string, want bool) { Expect(isPeerUser(name)).To(Equal(want)) },
		Entry("peer of the virtualization capability", "peering:peer:cluster-a:cap:virtualization", true),
		Entry("own controller", "system:serviceaccount:d8-virtualization:virtualization-controller", true),
		Entry("peer of another capability", "peering:peer:cluster-a:cap:mesh", false),
		Entry("no peer name", "peering:peer::cap:virtualization", false),
		Entry("user", "daniil@flant.com", false),
		Entry("other service account", "system:serviceaccount:default:daniil", false),
	)

	receiver := func(vm, instance string) *intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver {
		return &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Name: "web-01"},
			Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
				VirtualMachineTemplate:                       runtime.RawExtension{Raw: []byte(vm)},
				InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: []byte(instance)},
				IPAddress: "10.66.10.13",
			},
		}
	}
	DescribeTable("validateReceiver",
		func(vm, instance string, ok bool) {
			err := validateReceiver(receiver(vm, instance))
			if ok {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
		Entry("a VM with its domain", `{"spec":{"virtualMachineClassName":"generic"}}`, `{"spec":{"domain":{"devices":{"disks":[{"name":"vd-root"}]}}}}`, true),
		Entry("a VM of another name", `{"metadata":{"name":"db-01"}}`, `{"spec":{"domain":{"devices":{}}}}`, false),
		Entry("a host device", `{}`, `{"spec":{"domain":{"devices":{"hostDevices":[{"name":"x","deviceName":"y"}]}}}}`, false),
		Entry("a GPU", `{}`, `{"spec":{"domain":{"devices":{"gpus":[{"name":"x","deviceName":"y"}]}}}}`, false),
		Entry("not JSON", `{`, `{}`, false),
	)
})
