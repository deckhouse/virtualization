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

// Package pod provides a Pod-specialized observer.
package pod

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

type Observer = observer.Observer[*corev1.Pod]

type Predicate = observer.Predicate[*corev1.Pod]

func StartObserver(ctx context.Context, f *framework.Framework, pod *corev1.Pod) Observer {
	GinkgoHelper()

	obs, err := observer.New[*corev1.Pod](
		ctx,
		f.KubeClient().CoreV1().Pods(pod.Namespace),
		pod.Name,
		pod.Namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for Pod %s/%s", pod.Namespace, pod.Name)

	go failFastOnInvariant(obs, fmt.Sprintf("Pod %s/%s", pod.Namespace, pod.Name))

	DeferCleanup(func() {
		obs.Stop()
		Expect(obs.Err()).NotTo(HaveOccurred(),
			"Pod %s/%s observer reported an invariant violation",
			pod.Namespace, pod.Name)
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
