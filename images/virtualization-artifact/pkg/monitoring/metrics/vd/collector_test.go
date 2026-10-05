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
package vd

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func fqName(metric string) string {
	return metrics.MetricNamespace + "_" + metric
}

type stubIterator struct {
	disks []*v1alpha2.VirtualDisk
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, vd := range s.disks {
		if stop := h(newDataMetric(vd)); stop {
			return nil
		}
	}
	return nil
}

func collectorOf(disks ...*v1alpha2.VirtualDisk) Collector {
	return Collector{
		log:      log.NewNop(),
		iterator: stubIterator{disks: disks},
	}
}

// registryOf registers the collector the way the module does. The labels and annotations series
// carry labels the descriptor does not declare, which the pedantic registry behind
// testutil.CollectAndCompare rejects and the real one accepts.
func registryOf(c Collector) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	return reg
}

// newVD builds a disk without labels, annotations or consumers, so that the dynamic series do not
// obscure the fixed ones.
func newVD(name string, phase v1alpha2.DiskPhase) *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a", UID: types.UID("uid-" + name)},
		Status: v1alpha2.VirtualDiskStatus{
			Phase:            phase,
			Capacity:         "10Gi",
			StorageClassName: "linstor-r2",
			Target:           v1alpha2.DiskTarget{PersistentVolumeClaim: "vd-" + name + "-pvc"},
		},
	}
}

func inUse(status metav1.ConditionStatus) metav1.Condition {
	return metav1.Condition{Type: vdcondition.InUseType.String(), Status: status}
}

// phaseSeries renders the status_phase block of one disk with the current phase marked with 1. The
// phase list is diskPhases from the scraper on purpose: the scraper_test keeps it checked against
// the CRD enum, and this test keeps every phase of it reported.
func phaseSeries(name string, current v1alpha2.DiskPhase) string {
	var b strings.Builder
	b.WriteString("# HELP d8_virtualization_virtualdisk_status_phase The virtualdisk current phase.\n")
	b.WriteString("# TYPE d8_virtualization_virtualdisk_status_phase gauge\n")
	for _, p := range diskPhases {
		fmt.Fprintf(&b, "d8_virtualization_virtualdisk_status_phase{name=%q,namespace=\"team-a\",phase=%q,uid=%q} %v\n",
			name, string(p), "uid-"+name, common.BoolFloat64(p == current))
	}
	return b.String()
}

var _ = Describe("Collector", func() {
	// The whole snapshot rather than one metric: an accidental rename or label change fails here.
	It("reports every metric of a ready disk attached to a machine", func() {
		vd := newVD("vd-01", v1alpha2.DiskReady)
		vd.Status.Conditions = []metav1.Condition{inUse(metav1.ConditionTrue)}
		vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: "vm-01", Mounted: true}}
		c := collectorOf(vd)

		expected := `
# HELP d8_virtualization_virtualdisk_annotations Kubernetes annotations converted to Prometheus labels.
# TYPE d8_virtualization_virtualdisk_annotations gauge
d8_virtualization_virtualdisk_annotations{name="vd-01",namespace="team-a",uid="uid-vd-01"} 1
# HELP d8_virtualization_virtualdisk_capacity_bytes The virtualdisk capacity in bytes.
# TYPE d8_virtualization_virtualdisk_capacity_bytes gauge
d8_virtualization_virtualdisk_capacity_bytes{name="vd-01",namespace="team-a",uid="uid-vd-01"} 1.073741824e+10
# HELP d8_virtualization_virtualdisk_info Information about the virtualdisk.
# TYPE d8_virtualization_virtualdisk_info gauge
d8_virtualization_virtualdisk_info{name="vd-01",namespace="team-a",persistentvolumeclaim="vd-vd-01-pvc",storageclass="linstor-r2",uid="uid-vd-01"} 1
# HELP d8_virtualization_virtualdisk_labels Kubernetes labels converted to Prometheus labels.
# TYPE d8_virtualization_virtualdisk_labels gauge
d8_virtualization_virtualdisk_labels{name="vd-01",namespace="team-a",uid="uid-vd-01"} 1
# HELP d8_virtualization_virtualdisk_status_in_use Whether the virtualdisk is in use (1 - yes, 0 - no).
# TYPE d8_virtualization_virtualdisk_status_in_use gauge
d8_virtualization_virtualdisk_status_in_use{name="vd-01",namespace="team-a",uid="uid-vd-01",virtualmachine="vm-01"} 1
# HELP d8_virtualization_virtualdisk_status_phase The virtualdisk current phase.
# TYPE d8_virtualization_virtualdisk_status_phase gauge
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="Exporting",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="Failed",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="Migrating",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="PVCLost",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="Pending",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="Provisioning",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="Ready",uid="uid-vd-01"} 1
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="Resizing",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="Terminating",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="WaitForFirstConsumer",uid="uid-vd-01"} 0
d8_virtualization_virtualdisk_status_phase{name="vd-01",namespace="team-a",phase="WaitForUserUpload",uid="uid-vd-01"} 0
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected))).To(Succeed())
	})

	DescribeTable("marks exactly the current phase with 1",
		func(phase, marked v1alpha2.DiskPhase) {
			c := collectorOf(newVD("disk", phase))

			Expect(testutil.CollectAndCompare(c, strings.NewReader(phaseSeries("disk", marked)),
				fqName(MetricDiskStatusPhase))).To(Succeed())
		},
		Entry("Pending", v1alpha2.DiskPending, v1alpha2.DiskPending),
		Entry("WaitForUserUpload", v1alpha2.DiskWaitForUserUpload, v1alpha2.DiskWaitForUserUpload),
		Entry("WaitForFirstConsumer", v1alpha2.DiskWaitForFirstConsumer, v1alpha2.DiskWaitForFirstConsumer),
		Entry("Provisioning", v1alpha2.DiskProvisioning, v1alpha2.DiskProvisioning),
		Entry("Failed", v1alpha2.DiskFailed, v1alpha2.DiskFailed),
		Entry("PVCLost", v1alpha2.DiskLost, v1alpha2.DiskLost),
		Entry("Ready", v1alpha2.DiskReady, v1alpha2.DiskReady),
		Entry("Resizing", v1alpha2.DiskResizing, v1alpha2.DiskResizing),
		Entry("Exporting", v1alpha2.DiskExporting, v1alpha2.DiskExporting),
		Entry("Terminating", v1alpha2.DiskTerminating, v1alpha2.DiskTerminating),
		Entry("Migrating", v1alpha2.DiskMigrating, v1alpha2.DiskMigrating),
		// A disk the controller has not reached yet carries an empty phase. Reporting it as
		// Pending keeps it visible: no series at all looks like a broken exporter.
		Entry("an empty phase counts as Pending", v1alpha2.DiskPhase(""), v1alpha2.DiskPending),
	)

	// One series per machine while the disk is in use, and a single zero series with an empty
	// machine otherwise: the dashboards count the ones and the alerts match the empty label.
	DescribeTable("reports which machines use the disk",
		func(condition *metav1.Condition, attached []v1alpha2.AttachedVirtualMachine, expected string) {
			vd := newVD("vd-01", v1alpha2.DiskReady)
			if condition != nil {
				vd.Status.Conditions = []metav1.Condition{*condition}
			}
			vd.Status.AttachedToVirtualMachines = attached

			Expect(testutil.CollectAndCompare(collectorOf(vd), strings.NewReader(expected),
				fqName(MetricDiskStatusInUse))).To(Succeed())
		},
		Entry("nothing uses it", nil, nil, `
# HELP d8_virtualization_virtualdisk_status_in_use Whether the virtualdisk is in use (1 - yes, 0 - no).
# TYPE d8_virtualization_virtualdisk_status_in_use gauge
d8_virtualization_virtualdisk_status_in_use{name="vd-01",namespace="team-a",uid="uid-vd-01",virtualmachine=""} 0
`),
		Entry("two machines share it",
			&metav1.Condition{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue},
			[]v1alpha2.AttachedVirtualMachine{{Name: "vm-01", Mounted: true}, {Name: "vm-02"}}, `
# HELP d8_virtualization_virtualdisk_status_in_use Whether the virtualdisk is in use (1 - yes, 0 - no).
# TYPE d8_virtualization_virtualdisk_status_in_use gauge
d8_virtualization_virtualdisk_status_in_use{name="vd-01",namespace="team-a",uid="uid-vd-01",virtualmachine="vm-01"} 1
d8_virtualization_virtualdisk_status_in_use{name="vd-01",namespace="team-a",uid="uid-vd-01",virtualmachine="vm-02"} 1
`),
		// An image is being built from the disk: the condition is true, but no machine holds it.
		Entry("it is in use by something other than a machine",
			&metav1.Condition{Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue},
			nil, `
# HELP d8_virtualization_virtualdisk_status_in_use Whether the virtualdisk is in use (1 - yes, 0 - no).
# TYPE d8_virtualization_virtualdisk_status_in_use gauge
d8_virtualization_virtualdisk_status_in_use{name="vd-01",namespace="team-a",uid="uid-vd-01",virtualmachine=""} 0
`),
		// The machines are listed in the status, but the disk is not in use: the machines are stopped.
		Entry("the machines that reference it are stopped",
			&metav1.Condition{Type: vdcondition.InUseType.String(), Status: metav1.ConditionFalse},
			[]v1alpha2.AttachedVirtualMachine{{Name: "vm-01"}}, `
# HELP d8_virtualization_virtualdisk_status_in_use Whether the virtualdisk is in use (1 - yes, 0 - no).
# TYPE d8_virtualization_virtualdisk_status_in_use gauge
d8_virtualization_virtualdisk_status_in_use{name="vd-01",namespace="team-a",uid="uid-vd-01",virtualmachine=""} 0
`),
	)

	DescribeTable("reports the capacity in bytes",
		func(capacity, value string) {
			vd := newVD("vd-01", v1alpha2.DiskReady)
			vd.Status.Capacity = capacity

			expected := `
# HELP d8_virtualization_virtualdisk_capacity_bytes The virtualdisk capacity in bytes.
# TYPE d8_virtualization_virtualdisk_capacity_bytes gauge
d8_virtualization_virtualdisk_capacity_bytes{name="vd-01",namespace="team-a",uid="uid-vd-01"} ` + value + `
`
			Expect(testutil.CollectAndCompare(collectorOf(vd), strings.NewReader(expected),
				fqName(MetricDiskCapacityBytes))).To(Succeed())
		},
		Entry("a binary quantity", "10Gi", "1.073741824e+10"),
		Entry("a decimal quantity", "5G", "5e+09"),
		// The capacity is unknown until the PVC is bound; zero rather than a missing series keeps
		// the sum over a namespace defined.
		Entry("no capacity yet", "", "0"),
		Entry("a capacity that does not parse", "ten gigabytes", "0"),
	)

	// Kubernetes keys become Prometheus label names: the prefix tells a label from an annotation,
	// every character outside [a-zA-Z0-9_] turns into an underscore and camelCase becomes snake_case.
	It("turns the labels and annotations into labels of the labels and annotations series", func() {
		vd := newVD("vd-01", v1alpha2.DiskReady)
		vd.Labels = map[string]string{
			"app":                        "web",
			"team.example.com/ownerName": "ops",
		}
		vd.Annotations = map[string]string{
			"note": "keep",
			// The last applied configuration is a whole manifest: it never becomes a label.
			"kubectl.kubernetes.io/last-applied-configuration": "{}",
		}
		c := collectorOf(vd)

		expected := `
# HELP d8_virtualization_virtualdisk_annotations Kubernetes annotations converted to Prometheus labels.
# TYPE d8_virtualization_virtualdisk_annotations gauge
d8_virtualization_virtualdisk_annotations{annotation_note="keep",name="vd-01",namespace="team-a",uid="uid-vd-01"} 1
# HELP d8_virtualization_virtualdisk_labels Kubernetes labels converted to Prometheus labels.
# TYPE d8_virtualization_virtualdisk_labels gauge
d8_virtualization_virtualdisk_labels{label_app="web",label_team_example_com_owner_name="ops",name="vd-01",namespace="team-a",uid="uid-vd-01"} 1
`
		Expect(testutil.GatherAndCompare(registryOf(c), strings.NewReader(expected),
			fqName(MetricDiskLabels), fqName(MetricDiskAnnotations))).To(Succeed())
	})

	It("keeps the disks apart when several are reported", func() {
		first := newVD("first", v1alpha2.DiskReady)
		second := newVD("second", v1alpha2.DiskProvisioning)
		second.Status.StorageClassName = "ceph-rbd"
		second.Status.Target.PersistentVolumeClaim = ""
		c := collectorOf(first, second)

		expected := `
# HELP d8_virtualization_virtualdisk_info Information about the virtualdisk.
# TYPE d8_virtualization_virtualdisk_info gauge
d8_virtualization_virtualdisk_info{name="first",namespace="team-a",persistentvolumeclaim="vd-first-pvc",storageclass="linstor-r2",uid="uid-first"} 1
d8_virtualization_virtualdisk_info{name="second",namespace="team-a",persistentvolumeclaim="",storageclass="ceph-rbd",uid="uid-second"} 1
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected), fqName(MetricDiskInfo))).To(Succeed())
	})
})
