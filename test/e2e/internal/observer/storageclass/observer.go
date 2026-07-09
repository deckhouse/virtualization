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

// Package storageclass provides a StorageClass-specialized observer.
package storageclass

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

type Observer = observer.Observer[*storagev1.StorageClass]

type Predicate = observer.Predicate[*storagev1.StorageClass]

func StartObserver(ctx context.Context, f *framework.Framework, name string) Observer {
	GinkgoHelper()

	obs, err := observer.New[*storagev1.StorageClass](
		ctx,
		f.KubeClient().StorageV1().StorageClasses(),
		name,
		"",
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for StorageClass %q", name)

	go failFastOnInvariant(obs, fmt.Sprintf("StorageClass %q", name))

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"StorageClass %q observer reported an invariant violation", name)
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
