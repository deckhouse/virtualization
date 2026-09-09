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

// Package eventually is the only place in the e2e suite allowed to call
// Gomega's Eventually.
//
// Kubernetes resource lifecycle waits belong to the watch-based observers in
// internal/observer and must not be expressed through this package. What
// remains — and what this package exists for — are the states no watch can
// deliver:
//
//   - guest-side state read over SSH (services, processes, block devices,
//     network interfaces inside the VM);
//   - resources created asynchronously with generated names the test cannot
//     know in advance, discovered by listing;
//   - clients without watch support (e.g. the rewrite client for internal
//     VirtualMachineInstances);
//   - external endpoints (e.g. an HTTP upload URL).
//
// Tests express such waits through the generic primitives [Until],
// [UntilAssertion] and [UntilMatch], or through the shared domain helpers in
// this package, instead of calling Eventually directly.
package eventually

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
)

const defaultPolling = time.Second

type config struct {
	polling     time.Duration
	explanation []any
}

// Option customizes a single wait.
type Option func(*config)

// WithPolling overrides the default 1s polling interval.
func WithPolling(interval time.Duration) Option {
	return func(c *config) { c.polling = interval }
}

// WithExplanation attaches a Gomega failure explanation: a format string
// optionally followed by args, or a single func() string that is called only
// if the wait fails.
func WithExplanation(explanation ...any) Option {
	return func(c *config) { c.explanation = explanation }
}

// Until polls fn until it returns nil, failing the spec after timeout. A
// gomega.StopTrying error returned by fn aborts the wait immediately.
func Until(fn func() error, timeout time.Duration, options ...Option) {
	GinkgoHelper()
	UntilMatch(fn, Succeed(), timeout, options...)
}

// UntilAssertion polls fn until every assertion made on g passes, failing the
// spec after timeout.
func UntilAssertion(fn func(g Gomega), timeout time.Duration, options ...Option) {
	GinkgoHelper()
	UntilMatch(fn, Succeed(), timeout, options...)
}

// UntilMatch polls actual (anything Gomega's Eventually accepts: a value or a
// polled function) until matcher is satisfied, failing the spec after timeout.
func UntilMatch(actual any, matcher gomegatypes.GomegaMatcher, timeout time.Duration, options ...Option) {
	GinkgoHelper()

	cfg := config{polling: defaultPolling}
	for _, opt := range options {
		opt(&cfg)
	}

	Eventually(actual).WithTimeout(timeout).WithPolling(cfg.polling).Should(matcher, cfg.explanation...)
}
