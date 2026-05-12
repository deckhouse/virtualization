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
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type waitForPodStepStatStub struct {
	checkPodErr   error
	dvcrImageName string
	progress      string
}

func (s waitForPodStepStatStub) GetProgress(_ types.UID, _ *corev1.Pod, _ string, _ ...service.GetProgressOption) string {
	return s.progress
}

func (s waitForPodStepStatStub) GetDVCRImageName(_ *corev1.Pod) string {
	return s.dvcrImageName
}

func (s waitForPodStepStatStub) CheckPod(_ *corev1.Pod) error {
	return s.checkPodErr
}

var _ = Describe("WaitForPodStep", func() {
	DescribeTable("Take",
		func(
			pod *corev1.Pod,
			stat waitForPodStepStatStub,
			expectedErr error,
			expectedResult reconcile.Result,
			expectedPhase v1alpha2.ImagePhase,
			expectedReason string,
			expectedMessage string,
			expectedRegistryURL string,
			expectedProgress string,
		) {
			vi := &v1alpha2.VirtualImage{
				ObjectMeta: metav1.ObjectMeta{UID: types.UID("vi-uid")},
				Status: v1alpha2.VirtualImageStatus{
					Progress: "10%",
				},
			}
			cb := conditions.NewConditionBuilder(vicondition.ReadyType)

			result, err := NewWaitForPodStep(pod, nil, stat, cb).Take(context.Background(), vi)
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}

			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(expectedResult))
			Expect(vi.Status.Phase).To(Equal(expectedPhase))
			Expect(vi.Status.Target.RegistryURL).To(Equal(expectedRegistryURL))

			if expectedProgress == "" {
				Expect(vi.Status.Progress).To(Equal("10%"))
			} else {
				Expect(vi.Status.Progress).To(Equal(expectedProgress))
			}

			Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
			Expect(cb.Condition().Reason).To(Equal(expectedReason))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
		},
		Entry("waits when pod is absent",
			nil,
			waitForPodStepStatStub{},
			nil,
			reconcile.Result{},
			v1alpha2.ImageProvisioning,
			vicondition.Provisioning.String(),
			"Waiting for the importer pod to be created by controller.",
			"",
			"",
		),
		Entry("requeues when pvc is not yet bound",
			&corev1.Pod{},
			waitForPodStepStatStub{checkPodErr: fmt.Errorf("%w: pod has unbound immediate PersistentVolumeClaims", service.ErrNotInitialized)},
			nil,
			reconcile.Result{Requeue: true},
			v1alpha2.ImageProvisioning,
			vicondition.Provisioning.String(),
			"Waiting for PersistentVolumeClaim to be Bound",
			"",
			"",
		),
		Entry("fails when provisioning did not start",
			&corev1.Pod{},
			waitForPodStepStatStub{checkPodErr: fmt.Errorf("%w: waiting for init", service.ErrNotInitialized)},
			nil,
			reconcile.Result{},
			v1alpha2.ImageFailed,
			vicondition.ProvisioningNotStarted.String(),
			"Not initialized: waiting for init.",
			"",
			"",
		),
		Entry("fails when provisioning has failed",
			&corev1.Pod{},
			waitForPodStepStatStub{checkPodErr: fmt.Errorf("%w: importer failed", service.ErrProvisioningFailed)},
			nil,
			reconcile.Result{},
			v1alpha2.ImageFailed,
			vicondition.ProvisioningFailed.String(),
			"Provisioning failed: importer failed.",
			"",
			"",
		),
		Entry("returns unknown error",
			&corev1.Pod{},
			waitForPodStepStatStub{checkPodErr: errors.New("boom")},
			errors.New("boom"),
			reconcile.Result{},
			v1alpha2.ImageFailed,
			vicondition.ProvisioningFailed.String(),
			"Boom.",
			"",
			"",
		),
		Entry("waits while pod is not running",
			&corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}},
			waitForPodStepStatStub{dvcrImageName: "registry/image:pending"},
			nil,
			reconcile.Result{},
			v1alpha2.ImageProvisioning,
			vicondition.Provisioning.String(),
			"Preparing to start import to DVCR.",
			"registry/image:pending",
			"",
		),
		Entry("updates progress for running pod",
			&corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}},
			waitForPodStepStatStub{dvcrImageName: "registry/image:running", progress: "45%"},
			nil,
			reconcile.Result{RequeueAfter: 2 * time.Second},
			v1alpha2.ImageProvisioning,
			vicondition.Provisioning.String(),
			"Import is in the process of provisioning to DVCR.",
			"registry/image:running",
			"45%",
		),
	)
})
