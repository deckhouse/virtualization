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

package source

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	servicestat "github.com/deckhouse/virtualization-controller/pkg/controller/service/stat"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

func TestSources(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CVI Sources")
}

var _ = Describe("Sources", func() {
	newCVI := func() *v1alpha2.ClusterVirtualImage {
		return &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cvi",
				UID:  "cvi-uid",
			},
		}
	}

	DescribeTable(
		"setPhaseConditionFromPodError",
		func(
			inputErr error,
			expectedErr error,
			expectedReason string,
			expectedMessage string,
		) {
			cvi := newCVI()
			cb := conditions.NewConditionBuilder(cvicondition.ReadyType)

			err := setPhaseConditionFromPodError(cb, cvi, inputErr)
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}

			Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageFailed))
			Expect(cb.Condition().Reason).To(Equal(expectedReason))
			Expect(cb.Condition().Message).To(Equal(expectedMessage))
		},
		Entry("not initialized", servicestat.ErrNotInitialized, nil, cvicondition.ProvisioningNotStarted.String(), "Not initialized."),
		Entry("not scheduled", servicestat.ErrNotScheduled, nil, cvicondition.ProvisioningNotStarted.String(), "Not scheduled."),
		Entry("provisioning failed", servicestat.ErrProvisioningFailed, nil, cvicondition.ProvisioningFailed.String(), "Provisioning failed."),
		Entry("unknown error", errors.New("boom"), errors.New("boom"), conditions.ReasonUnknown.String(), ""),
	)

	DescribeTable(
		"recordProvisioningFailedEvent",
		func(inputErr error, expectEvent bool) {
			var recorded bool
			recorder := &eventrecord.EventRecorderLoggerMock{
				EventFunc: func(_ client.Object, eventType, reason, _ string) {
					recorded = true
					Expect(eventType).To(Equal(corev1.EventTypeWarning))
					Expect(reason).To(Equal(v1alpha2.ReasonDataSourceDiskProvisioningFailed))
				},
			}

			recordProvisioningFailedEvent(recorder, newCVI(), inputErr)

			Expect(recorded).To(Equal(expectEvent))
		},
		Entry("provisioning failed", servicestat.ErrProvisioningFailed, true),
		Entry("wrapped provisioning failed", errors.Join(servicestat.ErrProvisioningFailed, errors.New("details")), true),
		Entry("not initialized", servicestat.ErrNotInitialized, false),
		Entry("unknown error", errors.New("boom"), false),
	)
})
