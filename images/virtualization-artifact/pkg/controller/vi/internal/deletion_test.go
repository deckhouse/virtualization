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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

		handler := NewDeletionHandler(sources, nil)
		result, err := handler.Handle(context.Background(), vi)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Second))

		cond, ok := conditions.GetCondition(vicondition.DeletingType, vi.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vicondition.DeletionCleanupPending.String()))
		Expect(cond.Message).To(Equal("Waiting for PersistentVolumeClaim deletion default/vi."))
	})

	It("sets Deleting condition when protection finalizer blocks deletion", func() {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())

		now := metav1.Now()
		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vi",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerVIProtection},
			},
		}
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm-a",
				Namespace: "default",
			},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachineRunning,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{Kind: v1alpha2.ImageDevice, Name: "vi"},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()
		handler := NewDeletionHandler(source.NewSources(), client)
		result, err := handler.Handle(context.Background(), vi)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeZero())

		cond, ok := conditions.GetCondition(vicondition.DeletingType, vi.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vicondition.DeletionBlockedByProtection.String()))
		Expect(cond.Message).To(Equal("The VirtualImage is protected from deletion because it is attached to VirtualMachine vm-a."))
	})
})
