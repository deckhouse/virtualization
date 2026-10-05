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
package vmsop

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func fqName(metric string) string {
	return metrics.MetricNamespace + "_" + metric
}

type stubIterator struct {
	operations []*v1alpha2.VirtualMachineSnapshotOperation
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, op := range s.operations {
		if stop := h(newDataMetric(op)); stop {
			return nil
		}
	}
	return nil
}

func collectorOf(operations ...*v1alpha2.VirtualMachineSnapshotOperation) Collector {
	return Collector{
		log:      log.NewNop(),
		iterator: stubIterator{operations: operations},
	}
}

func newVMSOP(name string, phase v1alpha2.VMSOPPhase) *v1alpha2.VirtualMachineSnapshotOperation {
	return &v1alpha2.VirtualMachineSnapshotOperation{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a", UID: types.UID("uid-" + name)},
		Status:     v1alpha2.VirtualMachineSnapshotOperationStatus{Phase: phase},
	}
}

// operationPhases is every phase the metric must export a series for. A phase missing from the
// scraper zeroes out the operation in the phase picture, which is what the table below catches.
var operationPhases = []v1alpha2.VMSOPPhase{
	v1alpha2.VMSOPPhasePending,
	v1alpha2.VMSOPPhaseInProgress,
	v1alpha2.VMSOPPhaseCompleted,
	v1alpha2.VMSOPPhaseFailed,
	v1alpha2.VMSOPPhaseTerminating,
}

// phaseSeries renders the status_phase block of one operation with the current phase marked with 1.
func phaseSeries(name string, current v1alpha2.VMSOPPhase) string {
	var b strings.Builder
	b.WriteString("# HELP d8_virtualization_virtualmachinesnapshotoperation_status_phase The virtualmachinesnapshotoperation current phase.\n")
	b.WriteString("# TYPE d8_virtualization_virtualmachinesnapshotoperation_status_phase gauge\n")
	for _, p := range operationPhases {
		fmt.Fprintf(&b, "d8_virtualization_virtualmachinesnapshotoperation_status_phase{name=%q,namespace=\"team-a\",phase=%q,uid=%q} %v\n",
			name, string(p), "uid-"+name, common.BoolFloat64(p == current))
	}
	return b.String()
}

var _ = Describe("Collector", func() {
	// The whole snapshot rather than one metric: an accidental rename or label change fails here.
	It("reports every metric of an operation that has completed", func() {
		c := collectorOf(newVMSOP("restore-01", v1alpha2.VMSOPPhaseCompleted))

		expected := `
# HELP d8_virtualization_virtualmachinesnapshotoperation_status_phase The virtualmachinesnapshotoperation current phase.
# TYPE d8_virtualization_virtualmachinesnapshotoperation_status_phase gauge
d8_virtualization_virtualmachinesnapshotoperation_status_phase{name="restore-01",namespace="team-a",phase="Completed",uid="uid-restore-01"} 1
d8_virtualization_virtualmachinesnapshotoperation_status_phase{name="restore-01",namespace="team-a",phase="Failed",uid="uid-restore-01"} 0
d8_virtualization_virtualmachinesnapshotoperation_status_phase{name="restore-01",namespace="team-a",phase="InProgress",uid="uid-restore-01"} 0
d8_virtualization_virtualmachinesnapshotoperation_status_phase{name="restore-01",namespace="team-a",phase="Pending",uid="uid-restore-01"} 0
d8_virtualization_virtualmachinesnapshotoperation_status_phase{name="restore-01",namespace="team-a",phase="Terminating",uid="uid-restore-01"} 0
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected))).To(Succeed())
	})

	DescribeTable("marks exactly the current phase with 1",
		func(phase, marked v1alpha2.VMSOPPhase) {
			c := collectorOf(newVMSOP("operation", phase))

			Expect(testutil.CollectAndCompare(c, strings.NewReader(phaseSeries("operation", marked)),
				fqName(MetricVMSOPStatusPhase))).To(Succeed())
		},
		Entry("Pending", v1alpha2.VMSOPPhasePending, v1alpha2.VMSOPPhasePending),
		Entry("InProgress", v1alpha2.VMSOPPhaseInProgress, v1alpha2.VMSOPPhaseInProgress),
		Entry("Completed", v1alpha2.VMSOPPhaseCompleted, v1alpha2.VMSOPPhaseCompleted),
		Entry("Failed", v1alpha2.VMSOPPhaseFailed, v1alpha2.VMSOPPhaseFailed),
		Entry("Terminating", v1alpha2.VMSOPPhaseTerminating, v1alpha2.VMSOPPhaseTerminating),
		// An operation the controller has not reached yet carries an empty phase. Reporting it as
		// Pending keeps it visible: no series at all looks like a broken exporter.
		Entry("an empty phase counts as Pending", v1alpha2.VMSOPPhase(""), v1alpha2.VMSOPPhasePending),
	)

	It("keeps the operations apart when several are reported", func() {
		c := collectorOf(
			newVMSOP("first", v1alpha2.VMSOPPhaseCompleted),
			newVMSOP("second", v1alpha2.VMSOPPhaseInProgress),
		)

		Expect(testutil.CollectAndCount(c, fqName(MetricVMSOPStatusPhase))).To(Equal(2 * len(operationPhases)))
		Expect(testutil.CollectAndCompare(c, strings.NewReader(
			phaseSeries("first", v1alpha2.VMSOPPhaseCompleted)+
				strings.Join(strings.Split(phaseSeries("second", v1alpha2.VMSOPPhaseInProgress), "\n")[2:], "\n"),
		))).To(Succeed())
	})
})
