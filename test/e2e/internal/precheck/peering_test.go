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

package precheck

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

func TestPeeringPrecheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Peering precheck Suite")
}

var _ = Describe("Self-peer controller build precheck", func() {
	readyController := func(name string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: framework.ModuleNamespace, Labels: map[string]string{"app": "virtualization-controller"}},
			Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
		}
	}

	DescribeTable("checks build information from the actual controller binary", func(data, expectedError string) {
		kube := fake.NewClientset(readyController("controller"))
		calls := 0

		err := checkPeeringDevBuild(context.Background(), kube, func(context.Context, corev1.Pod) ([]byte, error) {
			calls++
			return []byte(data), nil
		})

		Expect(calls).To(Equal(1), "the deployed binary must be queried")

		if expectedError == "" {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(MatchError(ContainSubstring(expectedError)))
		}
	},
		Entry("accepts a DEV binary", `{"devBuild":true,"edition":"EE"}`, ""),
		Entry("rejects a release binary regardless of its image tag", `{"devBuild":false}`, "DEV_BUILD=true"),
		Entry("rejects missing build metadata", `{}`, "DEV_BUILD=true"),
		Entry("rejects empty output", "", "invalid build information"),
		Entry("rejects invalid JSON", "old controller", "invalid build information"),
		Entry("rejects an invalid field type", `{"devBuild":"true"}`, "invalid build information"),
	)

	It("rejects an older binary or an exec failure with an actionable error", func() {
		kube := fake.NewClientset(readyController("controller"))

		err := checkPeeringDevBuild(context.Background(), kube, func(context.Context, corev1.Pod) ([]byte, error) {
			return nil, errors.New("unknown flag: --print-build-info")
		})
		Expect(err).To(MatchError(And(ContainSubstring("DEV_BUILD=true"), ContainSubstring("unknown flag"))))
	})

	It("verifies every replica before accepting a DEV rollout", func() {
		kube := fake.NewClientset(readyController("controller-a"), readyController("controller-b"))

		var checked []string

		err := checkPeeringDevBuild(context.Background(), kube, func(_ context.Context, pod corev1.Pod) ([]byte, error) {
			checked = append(checked, pod.Name)
			return []byte(`{"devBuild":true}`), nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(checked).To(ConsistOf("controller-a", "controller-b"))
	})

	It("checks every active replica and rejects a mixed rollout", func() {
		kube := fake.NewClientset(readyController("controller-dev"), readyController("controller-release"))

		var checked []string

		err := checkPeeringDevBuild(context.Background(), kube, func(_ context.Context, pod corev1.Pod) ([]byte, error) {
			checked = append(checked, pod.Name)
			if pod.Name == "controller-release" {
				return []byte(`{"devBuild":false}`), nil
			}

			return []byte(`{"devBuild":true}`), nil
		})

		Expect(checked).To(ContainElement("controller-release"))
		Expect(err).To(MatchError(And(ContainSubstring("controller-release"), ContainSubstring("DEV_BUILD=true"))))
	})

	It("rejects a cluster without controller pods", func() {
		err := checkPeeringDevBuild(context.Background(), fake.NewClientset(), nil)
		Expect(err).To(MatchError(ContainSubstring("requires ready virtualization-controller pods")))
	})

	It("rejects an unready controller before executing a command", func() {
		pod := readyController("controller")
		pod.Status.Conditions = nil

		err := checkPeeringDevBuild(context.Background(), fake.NewClientset(pod), nil)
		Expect(err).To(MatchError(ContainSubstring("become ready")))
	})

	It("ignores a terminating replica and checks the replacement", func() {
		old := readyController("controller-old")
		now := metav1.Now()
		old.DeletionTimestamp = &now

		var checked []string

		err := checkPeeringDevBuild(context.Background(), fake.NewClientset(old, readyController("controller-new")), func(_ context.Context, pod corev1.Pod) ([]byte, error) {
			checked = append(checked, pod.Name)
			return []byte(`{"devBuild":true}`), nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(checked).To(Equal([]string{"controller-new"}))
	})
})
