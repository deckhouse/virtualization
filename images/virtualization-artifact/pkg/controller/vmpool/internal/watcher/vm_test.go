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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/expectations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("pool member watcher", func() {
	const (
		ns       = "default"
		poolName = "web"
		poolUID  = types.UID("pool-uid")
		vmUID    = types.UID("vm-uid")
	)
	key := types.NamespacedName{Namespace: ns, Name: poolName}

	var pool *v1alpha2.VirtualMachinePool
	BeforeEach(func() {
		pool = &v1alpha2.VirtualMachinePool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: ns, UID: poolUID}}
	})

	newQueue := func() workqueue.TypedRateLimitingInterface[reconcile.Request] {
		return workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	}
	memberVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:            poolName + "-a",
				Namespace:       ns,
				UID:             vmUID,
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(pool, v1alpha2.VirtualMachinePoolGVK)},
			},
		}
	}
	orphanVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "solo", Namespace: ns, UID: "solo-uid"}}
	}

	Describe("ownerKey", func() {
		It("returns the owning pool for a controlled member", func() {
			Expect(ownerKey(memberVM())).To(Equal(&key))
		})
		It("returns nil for a VM with no controller", func() {
			Expect(ownerKey(orphanVM())).To(BeNil())
		})
		It("returns nil for a VM controlled by another kind", func() {
			vm := orphanVM()
			owner := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "owner", UID: "o-uid"}}
			vm.OwnerReferences = []metav1.OwnerReference{
				*metav1.NewControllerRef(owner, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind)),
			}
			Expect(ownerKey(vm)).To(BeNil())
		})
	})

	Describe("event handlers", func() {
		var (
			exp *expectations.Expectations
			h   *memberEventHandler
			q   workqueue.TypedRateLimitingInterface[reconcile.Request]
		)
		BeforeEach(func() {
			exp = expectations.New()
			h = &memberEventHandler{exp: exp}
			q = newQueue()
		})

		It("observes the creation and enqueues the pool on a Create event", func() {
			exp.ExpectCreations(key.String(), 1)
			Expect(exp.Satisfied(key.String())).To(BeFalse())

			h.Create(context.Background(), event.TypedCreateEvent[*v1alpha2.VirtualMachine]{Object: memberVM()}, q)

			Expect(exp.Satisfied(key.String())).To(BeTrue()) // creation observed
			Expect(q.Len()).To(Equal(1))                     // pool re-enqueued
		})

		It("observes the deletion and enqueues the pool on a Delete event", func() {
			exp.ExpectDeletions(key.String(), vmUID)
			Expect(exp.Satisfied(key.String())).To(BeFalse())

			h.Delete(context.Background(), event.TypedDeleteEvent[*v1alpha2.VirtualMachine]{Object: memberVM()}, q)

			Expect(exp.Satisfied(key.String())).To(BeTrue()) // deletion observed
			Expect(q.Len()).To(Equal(1))
		})

		It("ignores a VM that is not owned by a pool", func() {
			exp.ExpectCreations(key.String(), 1)

			h.Create(context.Background(), event.TypedCreateEvent[*v1alpha2.VirtualMachine]{Object: orphanVM()}, q)

			Expect(exp.Satisfied(key.String())).To(BeFalse()) // expectation untouched
			Expect(q.Len()).To(Equal(0))                      // nothing enqueued
		})

		It("enqueues the pool on an Update event", func() {
			h.Update(context.Background(), event.TypedUpdateEvent[*v1alpha2.VirtualMachine]{ObjectOld: memberVM(), ObjectNew: memberVM()}, q)
			Expect(q.Len()).To(Equal(1))
		})
	})
})
