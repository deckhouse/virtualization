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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	clock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dlog "github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

var _ = Describe("CronSource", func() {
	const (
		// Every day at 00:00
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

		scheme := newScheme()
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		mgr = newFakeGCManager(fakeClient, 24*time.Hour, 10)

		t := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		fakeClock = clock.NewFakeClock(t)
	})

	newSource := func(scheduleSpec string) *CronSource {
		source, err := NewCronSource(scheduleSpec, NewObjectLister(mgr.ListForDelete), log)
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
		})

		AfterEach(func() {
			cancel()
			queue.ShutDown()
		})

		It("should not enqueue objects because ttl is not expired", func() {
			spawnFakeObjects(5, 10, fakeObjectPhaseCompleted, fakeClient, fakeClock)
			spawnFakeObjects(10, 10, fakeObjectPhasePending, fakeClient, fakeClock)
			spawnFakeObjects(15, 10, fakeObjectPhaseRunning, fakeClient, fakeClock)

			Expect(source.Start(ctx, queue)).To(Succeed())
			time.Sleep(1 * time.Second)

			// Go to 2025-01-02 01:00.
			// CronSource should be started but not enqueued any objects because ttl is not expired.
			fakeClock.Step(13 * time.Hour)

			Consistently(func() int {
				return len(queue.Requests())
			}).WithTimeout(10 * time.Second).Should(Equal(0))
		})

		It("should enqueue 10 objects because ttl is not expired but objects completed and them more that maxCount", func() {
			spawnFakeObjects(10, 11, fakeObjectPhaseCompleted, fakeClient, fakeClock)
			spawnFakeObjects(10, 11, fakeObjectPhasePending, fakeClient, fakeClock)
			spawnFakeObjects(10, 11, fakeObjectPhaseRunning, fakeClient, fakeClock)

			Expect(source.Start(ctx, queue)).To(Succeed())
			time.Sleep(1 * time.Second)

			// Go to 2025-01-02 01:00.
			// CronSource should be started and enqueued 10 objects.
			fakeClock.Step(13 * time.Hour)

			Eventually(func() int {
				return len(queue.Requests())
			}).WithTimeout(10 * time.Second).Should(Equal(10))
		})

		It("should enqueue 100 objects in completed state", func() {
			spawnFakeObjects(10, 10, fakeObjectPhaseCompleted, fakeClient, fakeClock)
			spawnFakeObjects(10, 10, fakeObjectPhasePending, fakeClient, fakeClock)
			spawnFakeObjects(10, 10, fakeObjectPhaseRunning, fakeClient, fakeClock)

			Expect(source.Start(ctx, queue)).To(Succeed())
			time.Sleep(1 * time.Second)

			// Go to 2025-01-03 01:00.
			// CronSource should be started and enqueued 100 objects.
			fakeClock.Step(37 * time.Hour)

			Eventually(func() int {
				return len(queue.Requests())
			}).WithTimeout(10 * time.Second).Should(Equal(100))
		})
	})
})
