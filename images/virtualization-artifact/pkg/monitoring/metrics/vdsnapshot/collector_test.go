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
package vdsnapshot

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
	snapshots []*v1alpha2.VirtualDiskSnapshot
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, vds := range s.snapshots {
		if stop := h(newDataMetric(vds)); stop {
			return nil
		}
	}
	return nil
}

func collectorOf(snapshots ...*v1alpha2.VirtualDiskSnapshot) Collector {
	return Collector{
		log:      log.NewNop(),
		iterator: stubIterator{snapshots: snapshots},
	}
}

func newVDSnapshot(name, diskName string, phase v1alpha2.VirtualDiskSnapshotPhase) *v1alpha2.VirtualDiskSnapshot {
	return &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a", UID: types.UID("uid-" + name)},
		Spec:       v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: diskName},
		Status:     v1alpha2.VirtualDiskSnapshotStatus{Phase: phase},
	}
}

// snapshotPhases is every phase the metric must export a series for. A phase missing from the
// scraper zeroes out the snapshot in the phase picture, which is what the table below catches.
var snapshotPhases = []v1alpha2.VirtualDiskSnapshotPhase{
	v1alpha2.VirtualDiskSnapshotPhasePending,
	v1alpha2.VirtualDiskSnapshotPhaseInProgress,
	v1alpha2.VirtualDiskSnapshotPhaseReady,
	v1alpha2.VirtualDiskSnapshotPhaseFailed,
	v1alpha2.VirtualDiskSnapshotPhaseTerminating,
}

// phaseSeries renders the status_phase block of one snapshot with the current phase marked with 1.
func phaseSeries(name string, current v1alpha2.VirtualDiskSnapshotPhase) string {
	var b strings.Builder
	b.WriteString("# HELP d8_virtualization_virtualdisksnapshot_status_phase The virtualdisksnapshot current phase.\n")
	b.WriteString("# TYPE d8_virtualization_virtualdisksnapshot_status_phase gauge\n")
	for _, p := range snapshotPhases {
		fmt.Fprintf(&b, "d8_virtualization_virtualdisksnapshot_status_phase{name=%q,namespace=\"team-a\",phase=%q,uid=%q} %v\n",
			name, string(p), "uid-"+name, common.BoolFloat64(p == current))
	}
	return b.String()
}

var _ = Describe("Collector", func() {
	// The whole snapshot rather than one metric: an accidental rename or label change fails here.
	It("reports every metric of a snapshot that is ready", func() {
		c := collectorOf(newVDSnapshot("vds-01", "vd-01", v1alpha2.VirtualDiskSnapshotPhaseReady))

		expected := `
# HELP d8_virtualization_virtualdisksnapshot_info The virtualdisksnapshot virtualdisk name.
# TYPE d8_virtualization_virtualdisksnapshot_info gauge
d8_virtualization_virtualdisksnapshot_info{name="vds-01",namespace="team-a",uid="uid-vds-01",virtualdisk="vd-01"} 1
# HELP d8_virtualization_virtualdisksnapshot_status_phase The virtualdisksnapshot current phase.
# TYPE d8_virtualization_virtualdisksnapshot_status_phase gauge
d8_virtualization_virtualdisksnapshot_status_phase{name="vds-01",namespace="team-a",phase="Failed",uid="uid-vds-01"} 0
d8_virtualization_virtualdisksnapshot_status_phase{name="vds-01",namespace="team-a",phase="InProgress",uid="uid-vds-01"} 0
d8_virtualization_virtualdisksnapshot_status_phase{name="vds-01",namespace="team-a",phase="Pending",uid="uid-vds-01"} 0
d8_virtualization_virtualdisksnapshot_status_phase{name="vds-01",namespace="team-a",phase="Ready",uid="uid-vds-01"} 1
d8_virtualization_virtualdisksnapshot_status_phase{name="vds-01",namespace="team-a",phase="Terminating",uid="uid-vds-01"} 0
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected))).To(Succeed())
	})

	DescribeTable("marks exactly the current phase with 1",
		func(phase, marked v1alpha2.VirtualDiskSnapshotPhase) {
			c := collectorOf(newVDSnapshot("snapshot", "disk", phase))

			Expect(testutil.CollectAndCompare(c, strings.NewReader(phaseSeries("snapshot", marked)),
				fqName(MetricVDSnapshotStatusPhase))).To(Succeed())
		},
		Entry("Pending", v1alpha2.VirtualDiskSnapshotPhasePending, v1alpha2.VirtualDiskSnapshotPhasePending),
		Entry("InProgress", v1alpha2.VirtualDiskSnapshotPhaseInProgress, v1alpha2.VirtualDiskSnapshotPhaseInProgress),
		Entry("Ready", v1alpha2.VirtualDiskSnapshotPhaseReady, v1alpha2.VirtualDiskSnapshotPhaseReady),
		Entry("Failed", v1alpha2.VirtualDiskSnapshotPhaseFailed, v1alpha2.VirtualDiskSnapshotPhaseFailed),
		Entry("Terminating", v1alpha2.VirtualDiskSnapshotPhaseTerminating, v1alpha2.VirtualDiskSnapshotPhaseTerminating),
		// A snapshot the controller has not reached yet carries an empty phase. Reporting it as
		// Pending keeps it visible: no series at all looks like a broken exporter.
		Entry("an empty phase counts as Pending", v1alpha2.VirtualDiskSnapshotPhase(""), v1alpha2.VirtualDiskSnapshotPhasePending),
	)

	// The info series is what joins a snapshot to its disk on the dashboards; a snapshot the
	// state-snapshotter core planned names the disk through sourceRef, and the label must follow.
	It("names the disk of a core-planned snapshot through its source reference", func() {
		vds := newVDSnapshot("vds-01", "", v1alpha2.VirtualDiskSnapshotPhaseReady)
		vds.Spec.SourceRef = &v1alpha2.UnifiedSnapshotterSpecSourceRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskKind,
			Name:       "vd-from-ref",
		}
		c := collectorOf(vds)

		expected := `
# HELP d8_virtualization_virtualdisksnapshot_info The virtualdisksnapshot virtualdisk name.
# TYPE d8_virtualization_virtualdisksnapshot_info gauge
d8_virtualization_virtualdisksnapshot_info{name="vds-01",namespace="team-a",uid="uid-vds-01",virtualdisk="vd-from-ref"} 1
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected), fqName(MetricVDSnapshotInfo))).To(Succeed())
	})

	It("keeps the snapshots apart when several are reported", func() {
		c := collectorOf(
			newVDSnapshot("first", "vd-a", v1alpha2.VirtualDiskSnapshotPhaseReady),
			newVDSnapshot("second", "vd-b", v1alpha2.VirtualDiskSnapshotPhaseInProgress),
		)

		expected := `
# HELP d8_virtualization_virtualdisksnapshot_info The virtualdisksnapshot virtualdisk name.
# TYPE d8_virtualization_virtualdisksnapshot_info gauge
d8_virtualization_virtualdisksnapshot_info{name="first",namespace="team-a",uid="uid-first",virtualdisk="vd-a"} 1
d8_virtualization_virtualdisksnapshot_info{name="second",namespace="team-a",uid="uid-second",virtualdisk="vd-b"} 1
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected), fqName(MetricVDSnapshotInfo))).To(Succeed())
	})
})
