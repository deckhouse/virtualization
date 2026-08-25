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

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

var _ = Describe("isNodeUnresponsive", func() {
	const nodeName = "worker-0"

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	newNode := func(ready corev1.ConditionStatus) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:               corev1.NodeReady,
					Status:             ready,
					LastTransitionTime: metav1.Now(),
				}},
			},
		}
	}

	It("reports responsive node as fine", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects(newNode(corev1.ConditionTrue))
		Expect(err).NotTo(HaveOccurred())

		unresponsive, message, err := isNodeUnresponsive(ctx, fakeClient, nodeName)
		Expect(err).NotTo(HaveOccurred())
		Expect(unresponsive).To(BeFalse())
		Expect(message).To(BeEmpty())
	})

	It("reports node with Ready=Unknown as unresponsive", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects(newNode(corev1.ConditionUnknown))
		Expect(err).NotTo(HaveOccurred())

		unresponsive, message, err := isNodeUnresponsive(ctx, fakeClient, nodeName)
		Expect(err).NotTo(HaveOccurred())
		Expect(unresponsive).To(BeTrue())
		Expect(message).To(ContainSubstring(nodeName))
	})

	It("reports missing node as unresponsive", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		unresponsive, message, err := isNodeUnresponsive(ctx, fakeClient, nodeName)
		Expect(err).NotTo(HaveOccurred())
		Expect(unresponsive).To(BeTrue())
		Expect(message).To(ContainSubstring("gone"))
	})

	It("skips an instance that is not assigned to any node", func() {
		fakeClient, err := testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		unresponsive, _, err := isNodeUnresponsive(ctx, fakeClient, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(unresponsive).To(BeFalse())
	})
})
