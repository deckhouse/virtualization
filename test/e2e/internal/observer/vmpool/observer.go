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

// Package vmpool provides a VirtualMachinePool-specialized
// [observer.Observer] together with predicates ready to be used with its
// Never, Always and WaitFor primitives.
package vmpool

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for VirtualMachinePools.
type Observer = observer.Observer[*v1alpha2.VirtualMachinePool]

// Predicate is a convenience type alias for the generic Predicate specialized
// for VirtualMachinePools.
type Predicate = observer.Predicate[*v1alpha2.VirtualMachinePool]

// StartObserver starts a VirtualMachinePool Observer for the given pool and
// registers a DeferCleanup that stops the underlying watch and asserts no
// registered invariant was violated. Start it before creating the pool so the
// very first status transitions are captured.
func StartObserver(ctx context.Context, f *framework.Framework, pool *v1alpha2.VirtualMachinePool) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.VirtualMachinePool](
		ctx,
		f.VirtClient().VirtualMachinePools(pool.Namespace),
		pool.Name,
		pool.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for VirtualMachinePool %s/%s", pool.Namespace, pool.Name)

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"VirtualMachinePool %s/%s observer reported an invariant violation",
			pool.Namespace, pool.Name)
	})

	return obs
}

// HaveReadyReplicas reports the pool's status counts exactly n ready members.
// The pool controller counts a non-terminating member with phase Running as
// ready, so this is the pool's own "N replicas are Running" contract.
// Intended for use with [Observer.WaitFor].
func HaveReadyReplicas(n int32) Predicate {
	return func(p *v1alpha2.VirtualMachinePool) (bool, error) {
		return p.Status.ReadyReplicas == n, nil
	}
}

// HaveReplicas reports the pool's status counts exactly n existing members
// (including Terminating ones). Intended for use with [Observer.WaitFor].
func HaveReplicas(n int32) Predicate {
	return func(p *v1alpha2.VirtualMachinePool) (bool, error) {
		return p.Status.Replicas == n, nil
	}
}

// HaveMoreReplicasThan reports an error as soon as the pool's status counts
// more than n existing members. Intended for use with [Observer.Never] to
// pin the pool size after a scale-down (e.g. no replacement must appear for
// a replica removed via scaleDownWith).
func HaveMoreReplicasThan(n int32) Predicate {
	return func(p *v1alpha2.VirtualMachinePool) (bool, error) {
		if p.Status.Replicas > n {
			return true, fmt.Errorf("pool %s/%s has %d replicas, expected at most %d", p.Namespace, p.Name, p.Status.Replicas, n)
		}
		return false, nil
	}
}
