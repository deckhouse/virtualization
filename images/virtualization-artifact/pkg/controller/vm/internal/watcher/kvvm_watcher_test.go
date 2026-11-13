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

package watcher

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func kvvm(kvvmAnnotations map[string]string) *virtv1.VirtualMachine {
	return &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Annotations: kvvmAnnotations}}
}

var _ = Describe("KVVM watcher", func() {
	DescribeTable("detects changes that require reconciling the VM",
		func(oldKVVM, newKVVM *virtv1.VirtualMachine, expected bool) {
			Expect(kvvmChanged(oldKVVM, newKVVM)).To(Equal(expected))
		},
		// The VM shows the move in its MovedToPeer condition, and the operation waits for it.
		Entry("move to a peer started",
			kvvm(nil), kvvm(map[string]string{annotations.AnnPeerMigrationVMOP: "op"}), true,
		),
		Entry("move to a peer ended",
			kvvm(map[string]string{annotations.AnnPeerMigrationVMOP: "op"}), kvvm(nil), true,
		),
		Entry("receive from a peer started",
			kvvm(nil),
			&virtv1.VirtualMachine{Spec: virtv1.VirtualMachineSpec{RunStrategy: ptr.To(virtv1.RunStrategyWaitAsReceiver)}},
			true,
		),
		Entry("receive from a peer ended",
			&virtv1.VirtualMachine{Spec: virtv1.VirtualMachineSpec{RunStrategy: ptr.To(virtv1.RunStrategyWaitAsReceiver)}},
			kvvm(nil), true,
		),
		Entry("start requested",
			kvvm(nil), kvvm(map[string]string{annotations.AnnVMStartRequested: "true"}), true,
		),
		Entry("unrelated annotation changed",
			kvvm(map[string]string{"example.com/note": "a"}), kvvm(map[string]string{"example.com/note": "b"}), false,
		),
		Entry("nothing changed",
			kvvm(map[string]string{annotations.AnnPeerMigrationVMOP: "op"}),
			kvvm(map[string]string{annotations.AnnPeerMigrationVMOP: "op"}), false,
		),
	)
})
