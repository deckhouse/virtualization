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

package step

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TerminatingStep", func() {
	now := metav1.NewTime(time.Now())

	DescribeTable("Take",
		func(pvc *corev1.PersistentVolumeClaim, expectResult bool) {
			result, err := NewTerminatingStep(pvc).Take(context.Background(), &v1alpha2.VirtualImage{})
			Expect(err).ToNot(HaveOccurred())

			if expectResult {
				Expect(result).ToNot(BeNil())
				Expect(result.IsZero()).To(BeFalse())
				return
			}

			Expect(result).To(BeNil())
		},
		Entry("returns nil when pvc is absent", nil, false),
		Entry("returns nil when pvc is not terminating", &corev1.PersistentVolumeClaim{}, false),
		Entry("requeues when pvc is terminating", &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}, true),
	)
})
