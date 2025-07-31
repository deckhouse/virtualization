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

package gc

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	clock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dlog "github.com/deckhouse/deckhouse/pkg/log"

	"github.com/deckhouse/virtualization-controller/pkg/common/pwgen"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

var _ = Describe("CronSource", func() {
	const (
		// Every day a 00:00
		scheduleSpec = "0 0 * * *"
	)

	var (
		log        *dlog.Logger
		baseCtx    context.Context
		fakeClient client.Client
		mgr        *fakeGCManager
		fakeClock  *clock.FakeClock
	)

	BeforeEach(func() {
		log = testutil.NewNoOpLogger()
		baseCtx = testutil.ToContext(context.Background(), log)

		scheme := apiruntime.NewScheme()
		for _, f := range []func(*apiruntime.Scheme) error{
			clientgoscheme.AddToScheme,
			AddToScheme,
		} {
			Expect(f(scheme)).To(Succeed())
		}

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		mgr = newFakeGCManager(fakeClient, 24*time.Hour, 10)
		t := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		fakeClock = clock.NewFakeClock(t)
	})

	newSource := func(scheduleSpec string) *CronSource {
		source, err := NewCronSource(scheduleSpec, fakeClient, mgr, log)
		Expect(err).NotTo(HaveOccurred())
		source.clock = fakeClock
		return source
	}

	Context("with spawned objects", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
			source *CronSource
			queue  *fakeQueue
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(baseCtx)
			source = newSource(scheduleSpec)
			queue = newFakeQueue()
			spawnFakeObjects(10, 10, fakeObjectPhaseCompleted, fakeClient, fakeClock)
			spawnFakeObjects(10, 10, fakeObjectPhasePending, fakeClient, fakeClock)
			spawnFakeObjects(10, 10, fakeObjectPhaseRunning, fakeClient, fakeClock)
		})

		AfterEach(func() {
			cancel()
			queue.ShutDown()
		})

		It("should not delete objects because ttl is not expired", func() {
			Expect(source.Start(ctx, queue)).To(Succeed())
			time.Sleep(1 * time.Second)

			// Go to 2025-01-02 01:00.
			// CronSource should be started but not enqueued any objects because ttl is not expired.
			fakeClock.Step(13 * time.Hour)
			Eventually(func() int {
				return len(queue.Requests())
			}).WithTimeout(10 * time.Second).Should(Equal(0))
		})

		It("should enqueue 100 objects in queue", func() {
			Expect(source.Start(ctx, queue)).To(Succeed())
			time.Sleep(1 * time.Second)

			// Go to 2025-01-03 01:00.
			// CronSource should be started and enqueued 100 objects.
			fakeClock.Step(37 * time.Hour)

			Eventually(func() int {
				return len(queue.Requests())
			}).WithTimeout(10 * time.Second).Should(Equal(100))
		})

		It("should enqueue 100 objects in completed state and delete them", func() {
			Expect(source.Start(ctx, queue)).To(Succeed())
			time.Sleep(1 * time.Second)

			// Go to 2025-01-03 01:00.
			// CronSource should be started and enqueued 100 objects.
			fakeClock.Step(37 * time.Hour)
			time.Sleep(1 * time.Second)
			fakeClock.Step(1 * time.Minute)

			Eventually(func() int {
				return len(queue.Requests())
			}).WithTimeout(10 * time.Second).Should(Equal(100))

			reconciler := NewReconciler(fakeClient, source, mgr)
			for _, req := range queue.Requests() {
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsZero()).To(BeTrue())
			}

			objs := &FakeObjectList{}
			Expect(fakeClient.List(ctx, objs)).To(Succeed())
			Expect(len(objs.Items)).To(Equal(200))
			for _, obj := range objs.Items {
				Expect(obj.Phase).NotTo(Equal(fakeObjectPhaseCompleted))
			}
		})
	})
})

func spawnFakeObjects(countNamespaces, countPerNamespace int, phase string, client client.Client, fakeClock *clock.FakeClock) {
	for i := 0; i < countNamespaces; i++ {
		namespace := fmt.Sprintf("test-namespace-%s-%d", pwgen.AlphaNum(32), i)
		for j := 0; j < countPerNamespace; j++ {
			obj := NewFakeObject(fmt.Sprintf("fake-%d", j), namespace)
			obj.CreationTimestamp = metav1.NewTime(fakeClock.Now())
			obj.Phase = phase
			Expect(client.Create(context.Background(), obj)).To(Succeed())
		}
	}
}

func newFakeQueue() *fakeQueue {
	limiter := workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]()
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(limiter, workqueue.TypedRateLimitingQueueConfig[reconcile.Request]{Name: "test"})
	return &fakeQueue{
		TypedRateLimitingInterface: queue,
	}
}

type fakeQueue struct {
	requests []reconcile.Request
	workqueue.TypedRateLimitingInterface[reconcile.Request]
}

func (q *fakeQueue) Add(req reconcile.Request) {
	q.requests = append(q.requests, req)
}

func (q *fakeQueue) Requests() []reconcile.Request {
	return q.requests
}
