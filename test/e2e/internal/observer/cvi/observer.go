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

// Package cvi provides a ClusterVirtualImage-specialized observer.
package cvi

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

type Observer = observer.Observer[*v1alpha2.ClusterVirtualImage]

type Predicate = observer.Predicate[*v1alpha2.ClusterVirtualImage]

func StartObserver(ctx context.Context, f *framework.Framework, cvi *v1alpha2.ClusterVirtualImage) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.ClusterVirtualImage](
		ctx,
		f.VirtClient().ClusterVirtualImages(),
		cvi.Name,
		cvi.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for ClusterVirtualImage %s", cvi.Name)

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"ClusterVirtualImage %s observer reported an invariant violation",
			cvi.Name)
	})

	return obs
}
