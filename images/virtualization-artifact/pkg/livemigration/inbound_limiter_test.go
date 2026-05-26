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

package livemigration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

var _ = Describe("InboundMigrationLimiter", func() {
	const (
		namespace  = "default"
		targetNode = "node-a"
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	newKVVMI := func(name, migrationUID string) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: virtv1.SchemeGroupVersion.String(),
				Kind:       virtv1.VirtualMachineInstanceGroupVersionKind.Kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: virtv1.VirtualMachineInstanceStatus{
				MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
					TargetNode:   targetNode,
					MigrationUID: types.UID(migrationUID),
				},
			},
		}
	}

	It("Should acquire one slot only for the same target node", func() {
		first := newKVVMI("first", "first-migration")
		second := newKVVMI("second", "second-migration")
		fakeClient, err := testutil.NewFakeClientWithObjects(first, second)
		Expect(err).NotTo(HaveOccurred())

		limiter := NewInboundMigrationLimiter(fakeClient)
		acquired, err := limiter.TryAcquire(ctx, first, targetNode, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(acquired).To(BeTrue())

		acquired, err = limiter.TryAcquire(ctx, second, targetNode, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(acquired).To(BeFalse())
	})

	It("Should release acquired slot", func() {
		first := newKVVMI("first", "first-migration")
		second := newKVVMI("second", "second-migration")
		fakeClient, err := testutil.NewFakeClientWithObjects(first, second)
		Expect(err).NotTo(HaveOccurred())

		limiter := NewInboundMigrationLimiter(fakeClient)
		acquired, err := limiter.TryAcquire(ctx, first, targetNode, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(acquired).To(BeTrue())

		Expect(limiter.Release(ctx, first, targetNode, 1)).To(Succeed())

		acquired, err = limiter.TryAcquire(ctx, second, targetNode, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(acquired).To(BeTrue())
	})

	It("Should steal stale slot", func() {
		first := newKVVMI("first", "first-migration")
		second := newKVVMI("second", "second-migration")
		fakeClient, err := testutil.NewFakeClientWithObjects(first, second)
		Expect(err).NotTo(HaveOccurred())

		limiter := NewInboundMigrationLimiter(fakeClient)
		acquired, err := limiter.TryAcquire(ctx, first, targetNode, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(acquired).To(BeTrue())

		first.Status.MigrationState.Completed = true
		Expect(fakeClient.Status().Update(ctx, first)).To(Succeed())

		acquired, err = limiter.TryAcquire(ctx, second, targetNode, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(acquired).To(BeTrue())

		var leases coordinationv1.LeaseList
		Expect(fakeClient.List(ctx, &leases, client.InNamespace(InboundMigrationLeaseNamespace))).To(Succeed())
		Expect(leases.Items).To(HaveLen(1))
		Expect(leases.Items[0].Annotations).To(HaveKeyWithValue(InboundMigrationVMINameAnnotation, second.Name))
	})
})
