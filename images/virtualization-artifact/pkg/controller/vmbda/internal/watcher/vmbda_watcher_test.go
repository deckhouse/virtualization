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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newAttachment(name, vmName string, kind v1alpha2.VMBDAObjectRefKind, bdName string) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	obj := &v1alpha2.VirtualMachineBlockDeviceAttachment{}
	obj.Name = name
	obj.Namespace = "ns"
	obj.Spec.VirtualMachineName = vmName
	obj.Spec.BlockDeviceRef = v1alpha2.VMBDAObjectRef{Kind: kind, Name: bdName}
	return obj
}

func requestNames(requests []reconcile.Request) []types.NamespacedName {
	names := make([]types.NamespacedName, 0, len(requests))
	for _, r := range requests {
		names = append(names, r.NamespacedName)
	}
	return names
}

var _ = Describe("VirtualMachineBlockDeviceAttachmentWatcher", func() {
	It("enqueues the other attachments of the block device of a deleted attachment", func() {
		deleted := newAttachment("old", "vm", v1alpha2.VMBDAObjectRefKindVirtualDisk, "vd")
		fakeClient, err := testutil.NewFakeClientWithObjects(
			newAttachment("same-vm", "vm", v1alpha2.VMBDAObjectRefKindVirtualDisk, "vd"),
			newAttachment("other-vm", "vm-2", v1alpha2.VMBDAObjectRefKindVirtualDisk, "vd"),
			newAttachment("other-disk", "vm", v1alpha2.VMBDAObjectRefKindVirtualDisk, "vd-2"),
			newAttachment("image-with-the-name", "vm", v1alpha2.VMBDAObjectRefKindVirtualImage, "vd"),
		)
		Expect(err).NotTo(HaveOccurred())

		requests := NewVirtualMachineBlockDeviceAttachmentWatcher(fakeClient).enqueueSameBlockDevice(context.Background(), deleted)

		Expect(requestNames(requests)).To(ConsistOf(
			types.NamespacedName{Namespace: "ns", Name: "same-vm"},
			types.NamespacedName{Namespace: "ns", Name: "other-vm"},
		))
	})
})
