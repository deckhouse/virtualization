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
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vdmetrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vd"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func observations(stage, datasource string) uint64 {
	ch := make(chan prometheus.Metric, 16)
	vdmetrics.ProvisioningDuration.Collect(ch)
	close(ch)

	for metric := range ch {
		m := &dto.Metric{}
		Expect(metric.Write(m)).To(Succeed())
		var gotStage, gotDataSource string
		for _, label := range m.GetLabel() {
			switch label.GetName() {
			case "stage":
				gotStage = label.GetValue()
			case "datasource":
				gotDataSource = label.GetValue()
			}
		}
		if gotStage == stage && gotDataSource == datasource {
			return m.GetHistogram().GetSampleCount()
		}
	}
	return 0
}

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
		vdmetrics.ProvisioningDuration.Reset()
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

	It("fills totalProvisioning once and observes the provisioning once", func() {
		current := newVD(100*time.Second, metav1.Condition{
			Type:               vdcondition.ReadyType.String(),
			Status:             metav1.ConditionTrue,
			Reason:             vdcondition.Ready.String(),
			LastTransitionTime: metav1.NewTime(time.Now()),
			ObservedGeneration: 1,
		})
		current.Status.Stats.CreationDuration.WaitingForDependencies = &metav1.Duration{Duration: 0}

		conflicted := current.DeepCopy()
		_, err := h.Handle(context.Background(), conflicted)
		Expect(err).NotTo(HaveOccurred())
		Expect(conflicted.Status.Stats.CreationDuration.TotalProvisioning).NotTo(BeNil())

		changed := current.DeepCopy()
		_, err = h.Handle(context.Background(), changed)
		Expect(err).NotTo(HaveOccurred())
		first := changed.Status.Stats.CreationDuration.TotalProvisioning.Duration
		ObserveCreationDuration(current, changed)

		Expect(observations(vdmetrics.ProvisioningStageProvisioning, vdmetrics.DataSourceBlank)).To(BeNumerically("==", 1),
			"only the write that succeeded feeds the histogram")
		Expect(observations(vdmetrics.ProvisioningStageWaitingForDependencies, vdmetrics.DataSourceBlank)).To(BeNumerically("==", 1),
			"a zero wait is a fast stage, not a missing one, and must be counted")

		current = changed.DeepCopy()
		changed.CreationTimestamp = metav1.NewTime(time.Now().Add(-500 * time.Second))
		_, err = h.Handle(context.Background(), changed)
		Expect(err).NotTo(HaveOccurred())
		ObserveCreationDuration(current, changed)

		Expect(changed.Status.Stats.CreationDuration.TotalProvisioning.Duration).To(Equal(first))
		Expect(observations(vdmetrics.ProvisioningStageProvisioning, vdmetrics.DataSourceBlank)).To(BeNumerically("==", 1),
			"the second reconciliation must not observe the provisioning again")
	})

	// The handler is not called here: a disk with a source runs into the importer, which this suite
	// has no services for.
	It("labels the observation with the data source of the disk", func() {
		current := newVD(3*time.Minute, metav1.Condition{
			Type:               vdcondition.ReadyType.String(),
			Status:             metav1.ConditionTrue,
			Reason:             vdcondition.Ready.String(),
			LastTransitionTime: metav1.NewTime(time.Now()),
			ObservedGeneration: 1,
		})
		current.Spec.DataSource = &v1alpha2.VirtualDiskDataSource{Type: v1alpha2.DataSourceTypeHTTP}

		changed := current.DeepCopy()
		changed.Status.Stats.CreationDuration.TotalProvisioning = &metav1.Duration{Duration: 3 * time.Minute}

		ObserveCreationDuration(current, changed)

		Expect(observations(vdmetrics.ProvisioningStageProvisioning, vdmetrics.DataSourceHTTP)).To(BeNumerically("==", 1))
		Expect(observations(vdmetrics.ProvisioningStageProvisioning, vdmetrics.DataSourceBlank)).To(BeNumerically("==", 0),
			"a disk with a source must not land in the series of the blank disks")
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
