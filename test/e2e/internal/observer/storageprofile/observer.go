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

// Package storageprofile provides a StorageProfile-specialized observer.
package storageprofile

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

type Observer = observer.Observer[*cdiv1beta1.StorageProfile]

type Predicate = observer.Predicate[*cdiv1beta1.StorageProfile]

func StartObserver(ctx context.Context, f *framework.Framework, name string) Observer {
	GinkgoHelper()

	gvr := rewrite.StorageProfile{}.GVR()
	w := &dynamicWatcher{
		client: f.DynamicClient(),
		gvr:    gvr,
	}

	obs, err := observer.New[*cdiv1beta1.StorageProfile](ctx, w, name, "")
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for StorageProfile %q", name)

	go failFastOnInvariant(obs, fmt.Sprintf("StorageProfile %q", name))

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"StorageProfile %q observer reported an invariant violation", name)
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
