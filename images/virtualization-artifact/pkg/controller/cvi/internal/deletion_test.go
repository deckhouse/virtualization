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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

// testCleanUpReason is a cleanup reason as produced by the image provisioning services.
var testCleanUpReason = service.CleanUpReason(service.CleanUpRoleImporterPod, types.NamespacedName{Namespace: "d8-virtualization", Name: "cvi-importer-cvi"})

var _ = Describe("DeletionHandler", func() {
	It("sets Terminating condition with cleanup pending reason", func() {
		now := metav1.Now()
		cvi := &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "cvi",
				DeletionTimestamp: &now,
			},
		}

		sources := source.NewSources()
		sources.Set(v1alpha2.DataSourceTypeHTTP, cviSourceHandler{
			cleanUp: func(context.Context, *v1alpha2.ClusterVirtualImage) (bool, string, error) {
				return true, testCleanUpReason, nil
			},
		})

		handler := NewDeletionHandler(sources, nil)
		result, err := handler.Handle(context.Background(), cvi)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Second))

		cond, ok := conditions.GetCondition(cvicondition.TerminatingType, cvi.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(cvicondition.CleanupPending.String()))
		Expect(cond.Message).To(Equal(service.CapitalizeFirstLetter(testCleanUpReason) + "."))
	})

	It("merges the cleanup reasons of every data source in a stable order", func() {
		now := metav1.Now()
		cvi := &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "cvi",
				DeletionTimestamp: &now,
			},
		}

		// The reasons are ordered by the data source type name, not by the (random) map
		// iteration order, so the message does not change from one reconcile to the next.
		reasonByType := map[v1alpha2.DataSourceType]string{
			v1alpha2.DataSourceTypeUpload:         "waiting for the uploader Pod to be deleted",
			v1alpha2.DataSourceTypeHTTP:           "waiting for the importer Pod to be deleted",
			v1alpha2.DataSourceTypeContainerImage: "waiting for the NetworkPolicy to be deleted",
		}

		sources := source.NewSources()
		for dsType, reason := range reasonByType {
			sources.Set(dsType, cviSourceHandler{
				cleanUp: func(context.Context, *v1alpha2.ClusterVirtualImage) (bool, string, error) {
					return true, reason, nil
				},
			})
		}

		handler := NewDeletionHandler(sources, nil)

		// A single reconcile proves nothing: with three data sources an unsorted
		// implementation still hits the expected order once every 3! = 6 calls, just by
		// the luck of the map iteration. Repeating makes such a streak (1/6^reconciles)
		// impossible in practice.
		const reconciles = 10

		for range reconciles {
			cvi.Status.Conditions = nil

			_, err := handler.Handle(context.Background(), cvi)
			Expect(err).NotTo(HaveOccurred())

			cond, ok := conditions.GetCondition(cvicondition.TerminatingType, cvi.Status.Conditions)
			Expect(ok).To(BeTrue())
			Expect(cond.Message).To(Equal(
				"Waiting for the NetworkPolicy to be deleted; " + // ContainerImage
					"waiting for the importer Pod to be deleted; " + // HTTP
					"waiting for the uploader Pod to be deleted."), // Upload
			)
		}
	})

	It("leaves the Terminating condition unset while the protection finalizer blocks deletion", func() {
		now := metav1.Now()
		cvi := &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "cvi",
				DeletionTimestamp: &now,
				Finalizers:        []string{v1alpha2.FinalizerCVIProtection},
			},
		}

		handler := NewDeletionHandler(source.NewSources(), nil)
		result, err := handler.Handle(context.Background(), cvi)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeZero())

		// The VirtualMachines holding the image are reported by the InUse condition instead.
		_, ok := conditions.GetCondition(cvicondition.TerminatingType, cvi.Status.Conditions)
		Expect(ok).To(BeFalse())
		Expect(cvi.Finalizers).To(ContainElement(v1alpha2.FinalizerCVIProtection))
	})
})

type cviSourceHandler struct {
	cleanUp func(context.Context, *v1alpha2.ClusterVirtualImage) (bool, string, error)
}

func (h cviSourceHandler) Sync(context.Context, *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (h cviSourceHandler) CleanUp(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (bool, string, error) {
	return h.cleanUp(ctx, cvi)
}

func (h cviSourceHandler) Validate(context.Context, *v1alpha2.ClusterVirtualImage) error {
	return nil
}
