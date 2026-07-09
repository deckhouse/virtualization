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

package backoff

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBackoff(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backoff Suite")
}

var _ = Describe("Progressive", func() {
	const (
		base = 15 * time.Second
		max  = 5 * time.Minute
	)

	It("returns base when elapsed is below base", func() {
		Expect(Progressive(0, base, max)).To(Equal(base))
		Expect(Progressive(5*time.Second, base, max)).To(Equal(base))
	})

	It("tracks elapsed between base and max", func() {
		Expect(Progressive(30*time.Second, base, max)).To(Equal(30 * time.Second))
		Expect(Progressive(2*time.Minute, base, max)).To(Equal(2 * time.Minute))
	})

	It("caps at max", func() {
		Expect(Progressive(10*time.Minute, base, max)).To(Equal(max))
	})
})
