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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

var _ = Describe("DeletionHandler", func() {
	It("sets Deleting condition with cleanup pending reason", func() {
		now := metav1.Now()
		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vi",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
		}

		sources := source.NewSources()
		sources.Set(v1alpha2.DataSourceTypeHTTP, &source.HandlerMock{
			CleanUpFunc: func(context.Context, *v1alpha2.VirtualImage) (bool, string, error) {
				return true, "waiting for PersistentVolumeClaim deletion default/vi", nil
			},
		})

		handler := NewDeletionHandler(sources)
		result, err := handler.Handle(context.Background(), vi)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Second))

		cond, ok := conditions.GetCondition(vicondition.DeletingType, vi.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vicondition.DeletionCleanupPending.String()))
		Expect(cond.Message).To(Equal("Waiting for PersistentVolumeClaim deletion default/vi."))
	})
})
