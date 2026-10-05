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
package vmbda

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
)

func fqName(metric string) string {
	return metrics.MetricNamespace + "_" + metric
}

type stubIterator struct {
	attachments []*v1alpha2.VirtualMachineBlockDeviceAttachment
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, vmbda := range s.attachments {
		if stop := h(newDataMetric(vmbda)); stop {
			return nil
		}
	}
	return nil
}

func collectorOf(attachments ...*v1alpha2.VirtualMachineBlockDeviceAttachment) Collector {
	return Collector{
		log:      log.NewNop(),
		iterator: stubIterator{attachments: attachments},
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

// newVMBDA builds an attachment without labels or annotations, so that the dynamic series do not
// obscure the fixed ones.
func newVMBDA(name string, phase v1alpha2.BlockDeviceAttachmentPhase) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return &v1alpha2.VirtualMachineBlockDeviceAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a", UID: types.UID("uid-" + name)},
		Status:     v1alpha2.VirtualMachineBlockDeviceAttachmentStatus{Phase: phase},
	}
}

// attachmentPhases is every phase the metric must export a series for. A phase missing from the
// scraper zeroes out the attachment in the phase picture, which is what the table below catches.
var attachmentPhases = []v1alpha2.BlockDeviceAttachmentPhase{
	v1alpha2.BlockDeviceAttachmentPhasePending,
	v1alpha2.BlockDeviceAttachmentPhaseInProgress,
	v1alpha2.BlockDeviceAttachmentPhaseAttached,
	v1alpha2.BlockDeviceAttachmentPhaseFailed,
	v1alpha2.BlockDeviceAttachmentPhaseTerminating,
}

// phaseSeries renders the status_phase block of one attachment with the current phase marked with 1.
func phaseSeries(name string, current v1alpha2.BlockDeviceAttachmentPhase) string {
	var b strings.Builder
	b.WriteString("# HELP d8_virtualization_virtualmachineblockdeviceattachment_status_phase The virtualmachineblockdeviceattachment current phase.\n")
	b.WriteString("# TYPE d8_virtualization_virtualmachineblockdeviceattachment_status_phase gauge\n")
	for _, p := range attachmentPhases {
		fmt.Fprintf(&b, "d8_virtualization_virtualmachineblockdeviceattachment_status_phase{name=%q,namespace=\"team-a\",phase=%q,uid=%q} %v\n",
			name, string(p), "uid-"+name, common.BoolFloat64(p == current))
	}
	return b.String()
}

var _ = Describe("Collector", func() {
	// The whole snapshot rather than one metric: an accidental rename or label change fails here.
	It("reports every metric of an attached device", func() {
		c := collectorOf(newVMBDA("vmbda-01", v1alpha2.BlockDeviceAttachmentPhaseAttached))

		expected := `
# HELP d8_virtualization_virtualmachineblockdeviceattachment_annotations Kubernetes annotations converted to Prometheus labels.
# TYPE d8_virtualization_virtualmachineblockdeviceattachment_annotations gauge
d8_virtualization_virtualmachineblockdeviceattachment_annotations{name="vmbda-01",namespace="team-a",uid="uid-vmbda-01"} 1
# HELP d8_virtualization_virtualmachineblockdeviceattachment_labels Kubernetes labels converted to Prometheus labels.
# TYPE d8_virtualization_virtualmachineblockdeviceattachment_labels gauge
d8_virtualization_virtualmachineblockdeviceattachment_labels{name="vmbda-01",namespace="team-a",uid="uid-vmbda-01"} 1
# HELP d8_virtualization_virtualmachineblockdeviceattachment_status_phase The virtualmachineblockdeviceattachment current phase.
# TYPE d8_virtualization_virtualmachineblockdeviceattachment_status_phase gauge
d8_virtualization_virtualmachineblockdeviceattachment_status_phase{name="vmbda-01",namespace="team-a",phase="Attached",uid="uid-vmbda-01"} 1
d8_virtualization_virtualmachineblockdeviceattachment_status_phase{name="vmbda-01",namespace="team-a",phase="Failed",uid="uid-vmbda-01"} 0
d8_virtualization_virtualmachineblockdeviceattachment_status_phase{name="vmbda-01",namespace="team-a",phase="InProgress",uid="uid-vmbda-01"} 0
d8_virtualization_virtualmachineblockdeviceattachment_status_phase{name="vmbda-01",namespace="team-a",phase="Pending",uid="uid-vmbda-01"} 0
d8_virtualization_virtualmachineblockdeviceattachment_status_phase{name="vmbda-01",namespace="team-a",phase="Terminating",uid="uid-vmbda-01"} 0
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected))).To(Succeed())
	})

	DescribeTable("marks exactly the current phase with 1",
		func(phase, marked v1alpha2.BlockDeviceAttachmentPhase) {
			c := collectorOf(newVMBDA("attachment", phase))

			Expect(testutil.CollectAndCompare(c, strings.NewReader(phaseSeries("attachment", marked)),
				fqName(MetricVMBDAStatusPhase))).To(Succeed())
		},
		Entry("Pending", v1alpha2.BlockDeviceAttachmentPhasePending, v1alpha2.BlockDeviceAttachmentPhasePending),
		Entry("InProgress", v1alpha2.BlockDeviceAttachmentPhaseInProgress, v1alpha2.BlockDeviceAttachmentPhaseInProgress),
		Entry("Attached", v1alpha2.BlockDeviceAttachmentPhaseAttached, v1alpha2.BlockDeviceAttachmentPhaseAttached),
		Entry("Failed", v1alpha2.BlockDeviceAttachmentPhaseFailed, v1alpha2.BlockDeviceAttachmentPhaseFailed),
		Entry("Terminating", v1alpha2.BlockDeviceAttachmentPhaseTerminating, v1alpha2.BlockDeviceAttachmentPhaseTerminating),
		// An attachment the controller has not reached yet carries an empty phase. Reporting it as
		// Pending keeps it visible: no series at all looks like a broken exporter.
		Entry("an empty phase counts as Pending", v1alpha2.BlockDeviceAttachmentPhase(""), v1alpha2.BlockDeviceAttachmentPhasePending),
	)

	// Kubernetes keys become Prometheus label names: the prefix tells a label from an annotation,
	// every character outside [a-zA-Z0-9_] turns into an underscore and camelCase becomes snake_case.
	It("turns the labels and annotations into labels of the labels and annotations series", func() {
		vmbda := newVMBDA("vmbda-01", v1alpha2.BlockDeviceAttachmentPhaseAttached)
		vmbda.Labels = map[string]string{
			"app":                        "web",
			"team.example.com/ownerName": "ops",
		}
		vmbda.Annotations = map[string]string{
			"note": "keep",
			// The last applied configuration is a whole manifest: it never becomes a label.
			"kubectl.kubernetes.io/last-applied-configuration": "{}",
		}
		c := collectorOf(vmbda)

		expected := `
# HELP d8_virtualization_virtualmachineblockdeviceattachment_annotations Kubernetes annotations converted to Prometheus labels.
# TYPE d8_virtualization_virtualmachineblockdeviceattachment_annotations gauge
d8_virtualization_virtualmachineblockdeviceattachment_annotations{annotation_note="keep",name="vmbda-01",namespace="team-a",uid="uid-vmbda-01"} 1
# HELP d8_virtualization_virtualmachineblockdeviceattachment_labels Kubernetes labels converted to Prometheus labels.
# TYPE d8_virtualization_virtualmachineblockdeviceattachment_labels gauge
d8_virtualization_virtualmachineblockdeviceattachment_labels{label_app="web",label_team_example_com_owner_name="ops",name="vmbda-01",namespace="team-a",uid="uid-vmbda-01"} 1
`
		Expect(testutil.GatherAndCompare(registryOf(c), strings.NewReader(expected),
			fqName(MetricVMBDALabels), fqName(MetricVMBDAAnnotations))).To(Succeed())
	})

	It("keeps the attachments apart when several are reported", func() {
		c := collectorOf(
			newVMBDA("first", v1alpha2.BlockDeviceAttachmentPhaseAttached),
			newVMBDA("second", v1alpha2.BlockDeviceAttachmentPhasePending),
		)

		expected := `
# HELP d8_virtualization_virtualmachineblockdeviceattachment_labels Kubernetes labels converted to Prometheus labels.
# TYPE d8_virtualization_virtualmachineblockdeviceattachment_labels gauge
d8_virtualization_virtualmachineblockdeviceattachment_labels{name="first",namespace="team-a",uid="uid-first"} 1
d8_virtualization_virtualmachineblockdeviceattachment_labels{name="second",namespace="team-a",uid="uid-second"} 1
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected), fqName(MetricVMBDALabels))).To(Succeed())
	})
})
