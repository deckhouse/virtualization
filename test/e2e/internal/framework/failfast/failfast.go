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

// Package failfast provides watch-based rules that fail the running spec as
// soon as an observed resource reaches a state it cannot recover from on its
// own, instead of letting the spec burn its whole wait timeout.
//
// Every rule (one per ff_*.go file) implements [FailFast]: the framework
// starts them in Before and cancels them via DeferCleanup before its own
// cleanup runs. A rule diagnoses objects with a [Match] function returning a
// [Finding]; a finding with a non-zero grace period is re-checked once after
// the grace elapses and only fails the spec if the object is still wedged, so
// transient states (a registry blip, a scheduler that has not caught up) do
// not produce false failures.
//
// A spec that deliberately drives a resource into a dead-end state registers
// it in the [Exemptions] shared by all rules of its framework: the rules then
// ignore the object and the pods it owns, and the spec asserts the expected
// failure on its own observer.
package failfast

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// FailFast is a single fail-fast rule.
type FailFast interface {
	// Start launches the rule's watcher in a background goroutine. The rule
	// runs until ctx is cancelled and fails the current spec (via ginkgo
	// Fail) on the first object that stays in a dead-end state. Objects
	// registered in exempt (and the pods they own) are ignored; a nil
	// exempt ignores nothing.
	Start(ctx context.Context, exempt *Exemptions)
}

// Finding is a diagnosed dead-end state of an observed object. A finding with
// a non-zero Grace is re-checked once after the grace elapses and only fails
// the spec if the object is still wedged. A finding with Skip set ends the
// spec as skipped instead of failed: the rule recognized a known environment
// or platform issue the spec cannot meaningfully verify against.
type Finding struct {
	Message string
	Grace   time.Duration
	Skip    bool
}

// Match diagnoses an observed object: nil means the object makes progress, a
// finding means it is wedged.
type Match[T observer.Object] func(T) *Finding

// Client is the minimum typed-client surface a rule needs: a watch over the
// collection for discovery and a point read for the post-grace re-check. The
// generated Kubernetes and virtualization clients satisfy it as-is.
type Client[T observer.Object] interface {
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Get(ctx context.Context, name string, opts metav1.GetOptions) (T, error)
}

// New builds a rule that watches the collection served by c and diagnoses
// every observed object with match. The subject prefixes the failure message
// right before the object name (e.g. "pod my-namespace/").
func New[T observer.Object](subject string, c Client[T], match Match[T]) FailFast {
	return &failFast[T]{subject: subject, client: c, match: match}
}

// watchWindow caps a single discovery pass; the rule's loop restarts the
// watch until the spec's cleanup cancels it.
const watchWindow = time.Hour

// defaultGrace is the standard re-check delay for findings that may still
// heal on their own (a scheduler catching up, a registry blip, a controller
// cache going stale). Anything still wedged after 30 seconds is not going to
// recover, so no rule waits longer.
const defaultGrace = 30 * time.Second

type failFast[T observer.Object] struct {
	subject string
	client  Client[T]
	match   Match[T]
	exempt  *Exemptions
}

func (ff *failFast[T]) Start(ctx context.Context, exempt *Exemptions) {
	ff.exempt = exempt
	go ff.run(ctx)
}

// run watches the collection until the spec's cleanup cancels ctx and fails
// the spec on the first object that stays wedged: a matched object with a
// non-zero grace is re-fetched once after the grace elapses, so a state that
// healed (or an object that was deleted) in the meantime keeps the spec
// running.
func (ff *failFast[T]) run(ctx context.Context) {
	defer GinkgoRecover()
	for ctx.Err() == nil {
		// The exemption check belongs in the watch predicate: an exempted
		// object wedged for good would otherwise match on every restart of
		// the watch and turn the loop below into a busy one.
		obj, err := observer.WaitForFirst(ctx, ff.client, watchWindow, func(o T) bool {
			return !ff.exempt.IsExempted(o) && ff.match(o) != nil
		})
		if err != nil {
			// The window elapsed, the watch was closed by the API server, or
			// the spec's cleanup cancelled ctx; the loop condition decides
			// whether to keep watching.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		finding := ff.match(obj)
		if finding == nil {
			continue
		}
		if finding.Grace > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(finding.Grace):
			}
			current, err := ff.client.Get(ctx, obj.GetName(), metav1.GetOptions{})
			if err != nil {
				// The object is gone; whatever it was wedged on died with it.
				continue
			}
			finding = ff.match(current)
			if finding == nil || ff.exempt.IsExempted(current) {
				continue
			}
		}
		if finding.Skip {
			Skip(fmt.Sprintf("fail-fast: %s%s %s", ff.subject, obj.GetName(), finding.Message))
			return
		}
		Fail(fmt.Sprintf("fail-fast: %s%s %s", ff.subject, obj.GetName(), finding.Message))
		return
	}
}
