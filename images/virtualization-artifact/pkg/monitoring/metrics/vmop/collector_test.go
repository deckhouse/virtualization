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
package vmop

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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func fqName(metric string) string {
	return metrics.MetricNamespace + "_" + metric
}

type stubIterator struct {
	operations []*v1alpha2.VirtualMachineOperation
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, vmop := range s.operations {
		if stop := h(newDataMetric(vmop)); stop {
			return nil
		}
	}
	return nil
}

func collectorOf(operations ...*v1alpha2.VirtualMachineOperation) Collector {
	return Collector{
		log:      log.NewNop(),
		iterator: stubIterator{operations: operations},
	}
}

const (
	createdAt  = 1700000000
	startedAt  = 1700000010
	finishedAt = 1700000025
)

// newVMOP builds an operation created at createdAt with the given conditions. Every operation in
// the cluster carries a creation timestamp, so the tests always set one.
func newVMOP(name string, opType v1alpha2.VMOPType, phase v1alpha2.VMOPPhase, conditions ...metav1.Condition) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "team-a",
			UID:               types.UID("uid-" + name),
			CreationTimestamp: metav1.Unix(createdAt, 0),
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{Type: opType, VirtualMachine: "vm-01"},
		Status: v1alpha2.VirtualMachineOperationStatus{
			Phase:      phase,
			Conditions: conditions,
		},
	}
}

func signalSent(status metav1.ConditionStatus) metav1.Condition {
	return metav1.Condition{
		Type:               vmopcondition.TypeSignalSent.String(),
		Status:             status,
		LastTransitionTime: metav1.Unix(startedAt, 0),
	}
}

func completed(status metav1.ConditionStatus, reason vmopcondition.ReasonCompleted) metav1.Condition {
	return metav1.Condition{
		Type:               vmopcondition.TypeCompleted.String(),
		Status:             status,
		Reason:             reason.String(),
		LastTransitionTime: metav1.Unix(finishedAt, 0),
	}
}

// operationPhases is every phase the metric must export a series for. A phase missing from the
// scraper zeroes out the operation in the phase picture, which is what the table below catches.
var operationPhases = []v1alpha2.VMOPPhase{
	v1alpha2.VMOPPhasePending,
	v1alpha2.VMOPPhaseInProgress,
	v1alpha2.VMOPPhaseCompleted,
	v1alpha2.VMOPPhaseFailed,
	v1alpha2.VMOPPhaseTerminating,
	v1alpha2.VMOPPhaseSuperseded,
}

// phaseSeries renders the status_phase block of one operation with the current phase marked with 1.
func phaseSeries(name string, current v1alpha2.VMOPPhase) string {
	var b strings.Builder
	b.WriteString("# HELP d8_virtualization_virtualmachineoperation_status_phase The virtualmachineoperation current phase.\n")
	b.WriteString("# TYPE d8_virtualization_virtualmachineoperation_status_phase gauge\n")
	for _, p := range operationPhases {
		fmt.Fprintf(&b, "d8_virtualization_virtualmachineoperation_status_phase{name=%q,namespace=\"team-a\",phase=%q,type=\"Restart\",uid=%q,virtualmachine=\"vm-01\"} %v\n",
			name, string(p), "uid-"+name, common.BoolFloat64(p == current))
	}
	return b.String()
}

var _ = Describe("Collector", func() {
	// The whole snapshot rather than one metric: an accidental rename or label change fails here.
	It("reports every metric of a completed operation", func() {
		c := collectorOf(newVMOP("restart-01", v1alpha2.VMOPTypeRestart, v1alpha2.VMOPPhaseCompleted,
			signalSent(metav1.ConditionTrue),
			completed(metav1.ConditionTrue, vmopcondition.ReasonOperationCompleted),
		))

		expected := `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="restart-01",namespace="team-a",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 1.7e+09
# HELP d8_virtualization_virtualmachineoperation_finished_timestamp_seconds The timestamp when virtualmachineoperation finished (Completed or Failed phase, Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_finished_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_finished_timestamp_seconds{name="restart-01",namespace="team-a",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 1.700000025e+09
# HELP d8_virtualization_virtualmachineoperation_started_timestamp_seconds The timestamp when virtualmachineoperation transitioned to InProgress phase (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_started_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_started_timestamp_seconds{name="restart-01",namespace="team-a",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 1.70000001e+09
# HELP d8_virtualization_virtualmachineoperation_status_phase The virtualmachineoperation current phase.
# TYPE d8_virtualization_virtualmachineoperation_status_phase gauge
d8_virtualization_virtualmachineoperation_status_phase{name="restart-01",namespace="team-a",phase="Completed",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 1
d8_virtualization_virtualmachineoperation_status_phase{name="restart-01",namespace="team-a",phase="Failed",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 0
d8_virtualization_virtualmachineoperation_status_phase{name="restart-01",namespace="team-a",phase="InProgress",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 0
d8_virtualization_virtualmachineoperation_status_phase{name="restart-01",namespace="team-a",phase="Pending",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 0
d8_virtualization_virtualmachineoperation_status_phase{name="restart-01",namespace="team-a",phase="Superseded",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 0
d8_virtualization_virtualmachineoperation_status_phase{name="restart-01",namespace="team-a",phase="Terminating",type="Restart",uid="uid-restart-01",virtualmachine="vm-01"} 0
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected))).To(Succeed())
	})

	DescribeTable("marks exactly the current phase with 1",
		func(phase, marked v1alpha2.VMOPPhase) {
			c := collectorOf(newVMOP("operation", v1alpha2.VMOPTypeRestart, phase))

			Expect(testutil.CollectAndCompare(c, strings.NewReader(phaseSeries("operation", marked)),
				fqName(MetricVMOPStatusPhase))).To(Succeed())
		},
		Entry("Pending", v1alpha2.VMOPPhasePending, v1alpha2.VMOPPhasePending),
		Entry("InProgress", v1alpha2.VMOPPhaseInProgress, v1alpha2.VMOPPhaseInProgress),
		Entry("Completed", v1alpha2.VMOPPhaseCompleted, v1alpha2.VMOPPhaseCompleted),
		Entry("Failed", v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseFailed),
		Entry("Terminating", v1alpha2.VMOPPhaseTerminating, v1alpha2.VMOPPhaseTerminating),
		Entry("Superseded", v1alpha2.VMOPPhaseSuperseded, v1alpha2.VMOPPhaseSuperseded),
		// An operation the controller has not reached yet carries an empty phase. Reporting it as
		// Pending keeps it visible: no series at all looks like a broken exporter.
		Entry("an empty phase counts as Pending", v1alpha2.VMOPPhase(""), v1alpha2.VMOPPhasePending),
	)

	// The three timestamps appear as the operation moves along, and a timestamp that has not
	// happened yet is a missing series, not a zero: time() minus zero would read as 55 years.
	DescribeTable("reports the timestamps the operation has reached",
		func(phase v1alpha2.VMOPPhase, conditions []metav1.Condition, expected string) {
			c := collectorOf(newVMOP("op", v1alpha2.VMOPTypeMigrate, phase, conditions...))

			Expect(testutil.CollectAndCompare(c, strings.NewReader(expected),
				fqName(MetricVMOPCreatedTimestamp),
				fqName(MetricVMOPStartedTimestamp),
				fqName(MetricVMOPFinishedTimestamp))).To(Succeed())
		},
		Entry("it is waiting to start", v1alpha2.VMOPPhasePending, nil, `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.7e+09
`),
		Entry("the signal has been sent", v1alpha2.VMOPPhaseInProgress,
			[]metav1.Condition{signalSent(metav1.ConditionTrue)}, `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.7e+09
# HELP d8_virtualization_virtualmachineoperation_started_timestamp_seconds The timestamp when virtualmachineoperation transitioned to InProgress phase (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_started_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_started_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.70000001e+09
`),
		// The condition is present but not yet true: the operation has not started.
		Entry("the signal is not sent yet", v1alpha2.VMOPPhaseInProgress,
			[]metav1.Condition{signalSent(metav1.ConditionFalse)}, `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.7e+09
`),
		Entry("the migration failed", v1alpha2.VMOPPhaseFailed,
			[]metav1.Condition{signalSent(metav1.ConditionTrue), completed(metav1.ConditionFalse, vmopcondition.ReasonOperationFailed)}, `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.7e+09
# HELP d8_virtualization_virtualmachineoperation_finished_timestamp_seconds The timestamp when virtualmachineoperation finished (Completed or Failed phase, Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_finished_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_finished_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.700000025e+09
# HELP d8_virtualization_virtualmachineoperation_started_timestamp_seconds The timestamp when virtualmachineoperation transitioned to InProgress phase (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_started_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_started_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.70000001e+09
`),
		// A newer operation of the same kind took over; the finish is the moment it was superseded.
		Entry("it was superseded", v1alpha2.VMOPPhaseSuperseded,
			[]metav1.Condition{completed(metav1.ConditionTrue, vmopcondition.ReasonSuperseded)}, `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.7e+09
# HELP d8_virtualization_virtualmachineoperation_finished_timestamp_seconds The timestamp when virtualmachineoperation finished (Completed or Failed phase, Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_finished_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_finished_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.700000025e+09
`),
		// The phase says Completed, but the condition still carries a transient reason: the
		// finish time is not known yet, and reporting the transition time of the condition would
		// date the finish to whenever the condition last changed.
		Entry("the phase is final but the condition is not", v1alpha2.VMOPPhaseCompleted,
			[]metav1.Condition{completed(metav1.ConditionFalse, vmopcondition.ReasonMigrationPending)}, `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.7e+09
`),
		// The terminal condition is set, but the phase has not caught up: no finish until it does.
		Entry("the condition is final but the phase is not", v1alpha2.VMOPPhaseInProgress,
			[]metav1.Condition{completed(metav1.ConditionTrue, vmopcondition.ReasonOperationCompleted)}, `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="op",namespace="team-a",type="Migrate",uid="uid-op",virtualmachine="vm-01"} 1.7e+09
`),
	)

	It("keeps the operations apart when several are reported", func() {
		c := collectorOf(
			newVMOP("first", v1alpha2.VMOPTypeStart, v1alpha2.VMOPPhaseCompleted),
			newVMOP("second", v1alpha2.VMOPTypeStop, v1alpha2.VMOPPhasePending),
		)

		expected := `
# HELP d8_virtualization_virtualmachineoperation_created_timestamp_seconds The timestamp when virtualmachineoperation was created (Unix timestamp in seconds).
# TYPE d8_virtualization_virtualmachineoperation_created_timestamp_seconds gauge
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="first",namespace="team-a",type="Start",uid="uid-first",virtualmachine="vm-01"} 1.7e+09
d8_virtualization_virtualmachineoperation_created_timestamp_seconds{name="second",namespace="team-a",type="Stop",uid="uid-second",virtualmachine="vm-01"} 1.7e+09
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected), fqName(MetricVMOPCreatedTimestamp))).To(Succeed())
	})
})
