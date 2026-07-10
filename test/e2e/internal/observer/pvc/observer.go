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

// Package pvc provides a PersistentVolumeClaim-specialized observer.
package pvc

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

type Observer = observer.Observer[*corev1.PersistentVolumeClaim]

type Predicate = observer.Predicate[*corev1.PersistentVolumeClaim]

func StartObserver(ctx context.Context, f *framework.Framework, claim *corev1.PersistentVolumeClaim) Observer {
	GinkgoHelper()

	obs, err := observer.New[*corev1.PersistentVolumeClaim](
		ctx,
		f.KubeClient().CoreV1().PersistentVolumeClaims(claim.Namespace),
		claim.Name,
		claim.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for PersistentVolumeClaim %s/%s", claim.Namespace, claim.Name)

	go failFastOnInvariant(obs, fmt.Sprintf("PersistentVolumeClaim %s/%s", claim.Namespace, claim.Name))

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"PersistentVolumeClaim %s/%s observer reported an invariant violation",
			claim.Namespace, claim.Name)
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
