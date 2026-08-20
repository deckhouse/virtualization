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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("StatsHandler", func() {
	var h StatsHandler

	// The disk has no data source, so the handler returns before the importer and the uploader are used.
	newVD := func(createdAgo time.Duration, ready metav1.Condition) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vd",
				Namespace:         "default",
				Generation:        1,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-createdAgo)),
			},
			Status: v1alpha2.VirtualDiskStatus{
				Conditions: []metav1.Condition{ready},
			},
		}
	}

	BeforeEach(func() {
		h = StatsHandler{}
	})

	It("calculates waitingForFirstConsumer while the disk waits for the virtual machine", func() {
		vd := newVD(time.Minute, metav1.Condition{
			Type:               vdcondition.ReadyType.String(),
			Status:             metav1.ConditionFalse,
			Reason:             vdcondition.WaitingForFirstConsumer.String(),
			LastTransitionTime: metav1.NewTime(time.Now().Add(-20 * time.Second)),
			ObservedGeneration: 1,
		})

		_, err := h.Handle(context.Background(), vd)
		Expect(err).NotTo(HaveOccurred())

		Expect(vd.Status.Stats.CreationDuration.WaitingForFirstConsumer).NotTo(BeNil())
		Expect(vd.Status.Stats.CreationDuration.WaitingForFirstConsumer.Duration).To(BeNumerically("~", 20*time.Second, 5*time.Second))
	})

	It("excludes waitingForFirstConsumer from totalProvisioning", func() {
		vd := newVD(100*time.Second, metav1.Condition{
			Type:               vdcondition.ReadyType.String(),
			Status:             metav1.ConditionTrue,
			Reason:             vdcondition.Ready.String(),
			LastTransitionTime: metav1.NewTime(time.Now()),
			ObservedGeneration: 1,
		})
		vd.Status.Stats.CreationDuration.WaitingForDependencies = &metav1.Duration{Duration: 10 * time.Second}
		vd.Status.Stats.CreationDuration.WaitingForFirstConsumer = &metav1.Duration{Duration: 30 * time.Second}

		_, err := h.Handle(context.Background(), vd)
		Expect(err).NotTo(HaveOccurred())

		Expect(vd.Status.Stats.CreationDuration.TotalProvisioning).NotTo(BeNil())
		Expect(vd.Status.Stats.CreationDuration.TotalProvisioning.Duration).To(BeNumerically("~", 60*time.Second, 5*time.Second))
	})

	It("does not change waitingForFirstConsumer once the disk leaves the state", func() {
		vd := newVD(time.Minute, metav1.Condition{
			Type:               vdcondition.ReadyType.String(),
			Status:             metav1.ConditionFalse,
			Reason:             vdcondition.Provisioning.String(),
			LastTransitionTime: metav1.NewTime(time.Now()),
			ObservedGeneration: 1,
		})
		vd.Status.Stats.CreationDuration.WaitingForFirstConsumer = &metav1.Duration{Duration: 30 * time.Second}

		_, err := h.Handle(context.Background(), vd)
		Expect(err).NotTo(HaveOccurred())

		Expect(vd.Status.Stats.CreationDuration.WaitingForFirstConsumer.Duration).To(Equal(30 * time.Second))
	})
})
