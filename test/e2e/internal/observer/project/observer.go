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

// Package project provides a Project-specialized observer. Project
// (deckhouse.io/v1alpha2) is not served by VirtClient, so the observer watches
// it through the dynamic client (see watcher.go).
package project

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dv1alpha2 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

type Observer = observer.Observer[*dv1alpha2.Project]

type Predicate = observer.Predicate[*dv1alpha2.Project]

func StartObserver(ctx context.Context, f *framework.Framework, name string) Observer {
	GinkgoHelper()

	w := &dynamicWatcher{
		client: f.DynamicClient(),
		gvr:    projectGVR,
	}

	obs, err := observer.New[*dv1alpha2.Project](ctx, w, name, "")
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for Project %q", name)

	go failFastOnInvariant(obs, fmt.Sprintf("Project %q", name))

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"Project %q observer reported an invariant violation", name)
	})

	return obs
}

func failFastOnInvariant(obs Observer, label string) {
	defer GinkgoRecover()
	select {
	case <-obs.InvariantViolated():
	case <-obs.Stopped():
	}
	if err := obs.Err(); err != nil {
		Fail(fmt.Sprintf("%s observer reported an invariant violation: %s", label, err))
	}
}
