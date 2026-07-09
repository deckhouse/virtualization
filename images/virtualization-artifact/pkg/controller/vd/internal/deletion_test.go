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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("DeletionHandler", func() {
	It("sets Deleting condition when protection finalizer blocks deletion", func() {
		now := metav1.Now()
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vd",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerVDProtection},
			},
			Status: v1alpha2.VirtualDiskStatus{
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
					{Name: "vm-a", Mounted: false},
					{Name: "vm-b", Mounted: true},
				},
			},
		}

		handler := NewDeletionHandler(source.NewSources(), nil)
		result, err := handler.Handle(context.Background(), vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		cond, ok := conditions.GetCondition(vdcondition.DeletingType, vd.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vdcondition.DeletionBlockedByProtection.String()))
		Expect(cond.Message).To(Equal("The VirtualDisk is protected from deletion because it is attached to VirtualMachine vm-b."))
	})

	It("sets Deleting condition with cleanup pending reason", func() {
		now := metav1.Now()
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vd",
				Namespace:         "default",
				DeletionTimestamp: &now,
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeHTTP,
				},
			},
		}

		sources := source.NewSources()
		sources.Set(v1alpha2.DataSourceTypeHTTP, &source.HandlerMock{
			CleanUpFunc: func(context.Context, *v1alpha2.VirtualDisk) (bool, string, error) {
				return true, "waiting for PersistentVolumeClaim deletion default/vd", nil
			},
		})

		handler := NewDeletionHandler(sources, nil)
		result, err := handler.Handle(context.Background(), vd)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Second))

		cond, ok := conditions.GetCondition(vdcondition.DeletingType, vd.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(vdcondition.DeletionCleanupPending.String()))
		Expect(cond.Message).To(Equal("Waiting for PersistentVolumeClaim deletion default/vd."))
	})
})
