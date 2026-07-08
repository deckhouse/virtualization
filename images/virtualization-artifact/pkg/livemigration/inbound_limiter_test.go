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
	"strconv"
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

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
		limiter := NewInboundMigrationLimiter(true, 1)
		Expect(limiter.TryAcquire(newKVVMI("first", "first-migration"), targetNode)).To(BeTrue())
		Expect(limiter.TryAcquire(newKVVMI("second", "second-migration"), targetNode)).To(BeFalse())
	})

	It("Should be idempotent for the same migration", func() {
		limiter := NewInboundMigrationLimiter(true, 1)
		first := newKVVMI("first", "first-migration")
		Expect(limiter.TryAcquire(first, targetNode)).To(BeTrue())
		Expect(limiter.TryAcquire(first, targetNode)).To(BeTrue())
		// The repeated acquire must not consume the only remaining slot.
		Expect(limiter.TryAcquire(newKVVMI("second", "second-migration"), targetNode)).To(BeFalse())
	})

	It("Should release the slot and let the next migration in", func() {
		limiter := NewInboundMigrationLimiter(true, 1)
		first := newKVVMI("first", "first-migration")
		Expect(limiter.TryAcquire(first, targetNode)).To(BeTrue())

		limiter.Release(first, targetNode)
		Expect(limiter.TryAcquire(newKVVMI("second", "second-migration"), targetNode)).To(BeTrue())
	})

	It("Should release idempotently", func() {
		limiter := NewInboundMigrationLimiter(true, 1)
		first := newKVVMI("first", "first-migration")
		Expect(limiter.TryAcquire(first, targetNode)).To(BeTrue())
		limiter.Release(first, targetNode)
		limiter.Release(first, targetNode)
		Expect(limiter.TryAcquire(newKVVMI("second", "second-migration"), targetNode)).To(BeTrue())
	})

	It("Should release the slot by VMI when the migration UID is unknown", func() {
		limiter := NewInboundMigrationLimiter(true, 1)
		first := newKVVMI("first", "first-migration")
		Expect(limiter.TryAcquire(first, targetNode)).To(BeTrue())
		Expect(limiter.TryAcquire(newKVVMI("second", "second-migration"), targetNode)).To(BeFalse())

		limiter.ReleaseByKVVMI(namespace, "first")
		Expect(limiter.TryAcquire(newKVVMI("second", "second-migration"), targetNode)).To(BeTrue())
	})

	It("Should not release a different VMI sharing a name prefix", func() {
		limiter := NewInboundMigrationLimiter(true, 2)
		Expect(limiter.TryAcquire(newKVVMI("vm", "vm-migration"), targetNode)).To(BeTrue())
		Expect(limiter.TryAcquire(newKVVMI("vm-2", "vm-2-migration"), targetNode)).To(BeTrue())

		limiter.ReleaseByKVVMI(namespace, "vm")
		// "vm-2" must still hold its slot: only one of the two is free now.
		Expect(limiter.TryAcquire(newKVVMI("third", "third-migration"), targetNode)).To(BeTrue())
		Expect(limiter.TryAcquire(newKVVMI("fourth", "fourth-migration"), targetNode)).To(BeFalse())
	})

	It("Should respect a limit greater than one", func() {
		limiter := NewInboundMigrationLimiter(true, 2)
		Expect(limiter.TryAcquire(newKVVMI("first", "first-migration"), targetNode)).To(BeTrue())
		Expect(limiter.TryAcquire(newKVVMI("second", "second-migration"), targetNode)).To(BeTrue())
		Expect(limiter.TryAcquire(newKVVMI("third", "third-migration"), targetNode)).To(BeFalse())
	})

	It("Should give each target node its own slots", func() {
		limiter := NewInboundMigrationLimiter(true, 1)
		Expect(limiter.TryAcquire(newKVVMI("first", "first-migration"), "node-a")).To(BeTrue())
		Expect(limiter.TryAcquire(newKVVMI("second", "second-migration"), "node-b")).To(BeTrue())
	})

	It("Should not acquire when disabled", func() {
		limiter := NewInboundMigrationLimiter(false, 1)
		Expect(limiter.Enabled()).To(BeFalse())
	})

	It("Should not hand out the same last slot to concurrent acquirers", func() {
		limiter := NewInboundMigrationLimiter(true, 1)

		const workers = 50
		var acquired int64
		var wg sync.WaitGroup
		wg.Add(workers)
		for i := range workers {
			go func(i int) {
				defer wg.Done()
				name := "vm-" + strconv.Itoa(i)
				if limiter.TryAcquire(newKVVMI(name, "migration-"+strconv.Itoa(i)), targetNode) {
					atomic.AddInt64(&acquired, 1)
				}
			}(i)
		}
		wg.Wait()

		Expect(acquired).To(Equal(int64(1)))
	})

	It("Should restore acquired slots from VMI annotations on startup", func() {
		acquiredVMI := newKVVMI("acquired", "acquired-migration")
		MarkInboundMigrationSlotAcquired(acquiredVMI, targetNode)

		waitingVMI := newKVVMI("waiting", "waiting-migration")
		MarkInboundMigrationSlotWaiting(waitingVMI, targetNode)

		staleVMI := newKVVMI("stale", "stale-migration")
		MarkInboundMigrationSlotAcquired(staleVMI, targetNode)
		staleVMI.Status.MigrationState.Completed = true

		fakeClient, err := testutil.NewFakeClientWithObjects(acquiredVMI, waitingVMI, staleVMI)
		Expect(err).NotTo(HaveOccurred())

		limiter := NewInboundMigrationLimiter(true, 1)
		Expect(limiter.Restore(ctx, fakeClient)).To(Succeed())

		// Only the acquired, still-active VMI must hold the single slot.
		Expect(limiter.TryAcquire(newKVVMI("newcomer", "newcomer-migration"), targetNode)).To(BeFalse())
		// The restored owner re-acquires idempotently.
		Expect(limiter.TryAcquire(acquiredVMI, targetNode)).To(BeTrue())
	})

	It("Should not restore when disabled", func() {
		acquiredVMI := newKVVMI("acquired", "acquired-migration")
		MarkInboundMigrationSlotAcquired(acquiredVMI, targetNode)

		fakeClient, err := testutil.NewFakeClientWithObjects(acquiredVMI)
		Expect(err).NotTo(HaveOccurred())

		limiter := NewInboundMigrationLimiter(false, 1)
		Expect(limiter.Restore(ctx, fakeClient)).To(Succeed())
	})
})
