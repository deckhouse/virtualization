/*
Copyright 2025 Flant JSC

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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

var _ = Describe("ImagePresenceHandler", func() {
	var (
		handler      *ImagePresenceHandler
		imageChecker *dvcr.ImageCheckerMock
	)

	BeforeEach(func() {
		imageChecker = &dvcr.ImageCheckerMock{}
		handler = NewImagePresenceHandler(imageChecker)
	})

	Context("Handle", func() {
		It("should skip if phase is not Ready", func() {
			cvi := &v1alpha2.ClusterVirtualImage{
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase: v1alpha2.ImagePending,
				},
			}

			result, err := handler.Handle(context.Background(), cvi)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImagePending))
			Expect(imageChecker.CheckImageExistsCalls()).To(BeEmpty())
		})

		It("should skip if registryURL is empty", func() {
			cvi := &v1alpha2.ClusterVirtualImage{
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase: v1alpha2.ImageReady,
					Target: v1alpha2.ClusterVirtualImageStatusTarget{
						RegistryURL: "",
					},
				},
			}

			result, err := handler.Handle(context.Background(), cvi)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageReady))
			Expect(imageChecker.CheckImageExistsCalls()).To(BeEmpty())
		})

		It("should set ImageLost phase when image does not exist", func() {
			imageChecker.CheckImageExistsFunc = func(_ context.Context, _ string) (bool, error) {
				return false, nil
			}

			cvi := &v1alpha2.ClusterVirtualImage{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase: v1alpha2.ImageReady,
					Target: v1alpha2.ClusterVirtualImageStatusTarget{
						RegistryURL: "dvcr.example.com/cvi/test:abc123",
					},
				},
			}

			result, err := handler.Handle(context.Background(), cvi)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageLost))

			readyCondition := findCondition(cvi.Status.Conditions, cvicondition.ReadyType.String())
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal(cvicondition.ImageLost.String()))
		})

		It("should keep Ready phase when image exists", func() {
			imageChecker.CheckImageExistsFunc = func(_ context.Context, _ string) (bool, error) {
				return true, nil
			}

			cvi := &v1alpha2.ClusterVirtualImage{
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase: v1alpha2.ImageReady,
					Target: v1alpha2.ClusterVirtualImageStatusTarget{
						RegistryURL: "dvcr.example.com/cvi/test:abc123",
					},
				},
			}

			result, err := handler.Handle(context.Background(), cvi)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageReady))
		})

		It("should return error when image check fails", func() {
			imageChecker.CheckImageExistsFunc = func(_ context.Context, _ string) (bool, error) {
				return false, errors.New("connection refused")
			}

			cvi := &v1alpha2.ClusterVirtualImage{
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase: v1alpha2.ImageReady,
					Target: v1alpha2.ClusterVirtualImageStatusTarget{
						RegistryURL: "dvcr.example.com/cvi/test:abc123",
					},
				},
			}

			result, err := handler.Handle(context.Background(), cvi)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
			Expect(result.RequeueAfter).To(BeZero())
			Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageReady))
		})
	})
})

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
