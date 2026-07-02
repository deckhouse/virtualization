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

package expectations

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
)

const key = "ci/web"

// referenceTime is an arbitrary fixed clock; the TTL test advances it by hand
// via the injected now func, so the real-world date is irrelevant.
var referenceTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

var _ = Describe("Expectations", func() {
	Context("an unknown key", func() {
		It("is satisfied (nothing expected yet)", func() {
			e := New()
			Expect(e.Satisfied(key)).To(BeTrue())
		})
	})

	Context("creations", func() {
		It("is unsatisfied until every expected creation is observed", func() {
			e := New()
			e.ExpectCreations(key, 2)
			Expect(e.Satisfied(key)).To(BeFalse())

			e.CreationObserved(key)
			Expect(e.Satisfied(key)).To(BeFalse())

			e.CreationObserved(key)
			Expect(e.Satisfied(key)).To(BeTrue())
		})

		It("does not bank surplus observations below zero", func() {
			e := New()
			e.ExpectCreations(key, 1)
			// Observe more than expected — the extra observations must be ignored.
			e.CreationObserved(key)
			e.CreationObserved(key)
			e.CreationObserved(key)
			Expect(e.Satisfied(key)).To(BeTrue())

			// A fresh expectation must not be pre-satisfied by earlier surplus.
			e.ExpectCreations(key, 1)
			Expect(e.Satisfied(key)).To(BeFalse())
		})

		It("ignores non-positive counts", func() {
			e := New()
			e.ExpectCreations(key, 0)
			e.ExpectCreations(key, -3)
			Expect(e.Satisfied(key)).To(BeTrue())
		})
	})

	Context("deletions", func() {
		uidA := types.UID("a")
		uidB := types.UID("b")

		It("is unsatisfied until every expected UID is observed deleted", func() {
			e := New()
			e.ExpectDeletions(key, []types.UID{uidA, uidB})
			Expect(e.Satisfied(key)).To(BeFalse())

			e.DeletionObserved(key, uidA)
			Expect(e.Satisfied(key)).To(BeFalse())

			e.DeletionObserved(key, uidB)
			Expect(e.Satisfied(key)).To(BeTrue())
		})

		It("is not fooled by duplicate or unrelated deletion events", func() {
			e := New()
			e.ExpectDeletions(key, []types.UID{uidA})

			// An unrelated UID must not satisfy the expectation.
			e.DeletionObserved(key, types.UID("unrelated"))
			Expect(e.Satisfied(key)).To(BeFalse())

			e.DeletionObserved(key, uidA)
			Expect(e.Satisfied(key)).To(BeTrue())

			// A duplicate delete event must not underflow anything.
			e.DeletionObserved(key, uidA)
			Expect(e.Satisfied(key)).To(BeTrue())
		})
	})

	Context("creations and deletions together", func() {
		It("requires both to be cleared", func() {
			e := New()
			e.ExpectCreations(key, 1)
			e.ExpectDeletions(key, []types.UID{"x"})

			e.CreationObserved(key)
			Expect(e.Satisfied(key)).To(BeFalse()) // deletion still pending

			e.DeletionObserved(key, "x")
			Expect(e.Satisfied(key)).To(BeTrue())
		})
	})

	Context("TTL safety valve", func() {
		It("becomes satisfied once the expectation outlives the TTL", func() {
			e := NewWithTTL(time.Minute)
			now := referenceTime
			e.now = func() time.Time { return now }

			e.ExpectCreations(key, 1)
			Expect(e.Satisfied(key)).To(BeFalse())

			// Just under the TTL — still honoured.
			now = now.Add(59 * time.Second)
			Expect(e.Satisfied(key)).To(BeFalse())

			// Past the TTL — treated as satisfied even without observation.
			now = now.Add(2 * time.Second)
			Expect(e.Satisfied(key)).To(BeTrue())
		})
	})

	Context("Forget", func() {
		It("drops the tracked expectation", func() {
			e := New()
			e.ExpectCreations(key, 3)
			Expect(e.Satisfied(key)).To(BeFalse())

			e.Forget(key)
			Expect(e.Satisfied(key)).To(BeTrue())
		})
	})

	Context("concurrent access", func() {
		It("is race-free under parallel expect/observe", func() {
			e := New()
			const workers = 16
			const perWorker = 200

			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					for i := 0; i < perWorker; i++ {
						e.ExpectCreations(key, 1)
						e.CreationObserved(key)
					}
				}()
			}
			wg.Wait()

			// Every creation was observed, so the tracker must settle satisfied.
			Expect(e.Satisfied(key)).To(BeTrue())
		})
	})
})
