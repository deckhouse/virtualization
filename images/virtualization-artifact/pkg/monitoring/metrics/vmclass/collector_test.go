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

package vmclass

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func fqName(metric string) string {
	return metrics.MetricNamespace + "_" + metric
}

type stubIterator struct {
	classes []*v1alpha2.VirtualMachineClass
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, c := range s.classes {
		if stop := h(newDataMetric(c)); stop {
			return nil
		}
	}
	return nil
}

func newVMClass(name string, cpuType v1alpha2.CPUType, phase v1alpha2.VirtualMachineClassPhase, nodes ...string) *v1alpha2.VirtualMachineClass {
	return &v1alpha2.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("uid-" + name)},
		Spec:       v1alpha2.VirtualMachineClassSpec{CPU: v1alpha2.CPU{Type: cpuType}},
		Status: v1alpha2.VirtualMachineClassStatus{
			Phase:          phase,
			AvailableNodes: nodes,
		},
	}
}

func collectorOf(classes ...*v1alpha2.VirtualMachineClass) Collector {
	return Collector{
		log:      log.NewNop(),
		iterator: stubIterator{classes: classes},
	}
}

var _ = Describe("Collector", func() {
	It("reports every metric of a class that is ready", func() {
		c := collectorOf(newVMClass("generic", v1alpha2.CPUTypeModel, v1alpha2.ClassPhaseReady, "node-1", "node-2"))

		expected := `
# HELP d8_virtualization_virtualmachineclass_available_node The node that supports the CPU model of this virtualmachineclass. One series per node.
# TYPE d8_virtualization_virtualmachineclass_available_node gauge
d8_virtualization_virtualmachineclass_available_node{name="generic",node="node-1",uid="uid-generic"} 1
d8_virtualization_virtualmachineclass_available_node{name="generic",node="node-2",uid="uid-generic"} 1
# HELP d8_virtualization_virtualmachineclass_info The virtualmachineclass details: the requested CPU type.
# TYPE d8_virtualization_virtualmachineclass_info gauge
d8_virtualization_virtualmachineclass_info{name="generic",type="Model",uid="uid-generic"} 1
# HELP d8_virtualization_virtualmachineclass_status_phase The virtualmachineclass current phase.
# TYPE d8_virtualization_virtualmachineclass_status_phase gauge
d8_virtualization_virtualmachineclass_status_phase{name="generic",phase="Pending",uid="uid-generic"} 0
d8_virtualization_virtualmachineclass_status_phase{name="generic",phase="Ready",uid="uid-generic"} 1
d8_virtualization_virtualmachineclass_status_phase{name="generic",phase="Terminating",uid="uid-generic"} 0
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected))).To(Succeed())
	})

	DescribeTable("marks exactly the current phase with 1",
		func(phase v1alpha2.VirtualMachineClassPhase, expected string) {
			c := collectorOf(newVMClass("class", v1alpha2.CPUTypeModel, phase, "node-1"))
			Expect(testutil.CollectAndCompare(c, strings.NewReader(expected), fqName(MetricVMClassStatusPhase))).To(Succeed())
		},
		Entry("Ready", v1alpha2.ClassPhaseReady, `
# HELP d8_virtualization_virtualmachineclass_status_phase The virtualmachineclass current phase.
# TYPE d8_virtualization_virtualmachineclass_status_phase gauge
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Pending",uid="uid-class"} 0
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Ready",uid="uid-class"} 1
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Terminating",uid="uid-class"} 0
`),
		Entry("Terminating", v1alpha2.ClassPhaseTerminating, `
# HELP d8_virtualization_virtualmachineclass_status_phase The virtualmachineclass current phase.
# TYPE d8_virtualization_virtualmachineclass_status_phase gauge
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Pending",uid="uid-class"} 0
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Ready",uid="uid-class"} 0
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Terminating",uid="uid-class"} 1
`),
		// A class the controller has not reached yet carries an empty phase. Reporting it as
		// Pending keeps the alert anchored: a class with no series at all is indistinguishable
		// from a broken exporter.
		Entry("an empty phase counts as Pending", v1alpha2.VirtualMachineClassPhase(""), `
# HELP d8_virtualization_virtualmachineclass_status_phase The virtualmachineclass current phase.
# TYPE d8_virtualization_virtualmachineclass_status_phase gauge
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Pending",uid="uid-class"} 1
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Ready",uid="uid-class"} 0
d8_virtualization_virtualmachineclass_status_phase{name="class",phase="Terminating",uid="uid-class"} 0
`),
	)

	// This is the case the D8VirtualizationVirtualMachineClassHasNoNodes alert is built on:
	// the phase says Ready, and the absence of node series is the only thing that gives the
	// class away. The alert therefore uses `unless` and not `== 0` — there is no series to
	// compare with zero.
	It("reports no node series for a class whose node list is empty", func() {
		c := collectorOf(newVMClass("host-class", v1alpha2.CPUTypeHost, v1alpha2.ClassPhaseReady))

		Expect(testutil.CollectAndCount(c, fqName(MetricVMClassAvailableNode))).To(BeZero())
		Expect(testutil.CollectAndCount(c, fqName(MetricVMClassStatusPhase))).To(Equal(3))
	})

	It("keeps the classes apart when several are reported", func() {
		c := collectorOf(
			newVMClass("first", v1alpha2.CPUTypeModel, v1alpha2.ClassPhaseReady, "node-1"),
			newVMClass("second", v1alpha2.CPUTypeDiscovery, v1alpha2.ClassPhasePending),
		)

		expected := `
# HELP d8_virtualization_virtualmachineclass_info The virtualmachineclass details: the requested CPU type.
# TYPE d8_virtualization_virtualmachineclass_info gauge
d8_virtualization_virtualmachineclass_info{name="first",type="Model",uid="uid-first"} 1
d8_virtualization_virtualmachineclass_info{name="second",type="Discovery",uid="uid-second"} 1
`
		Expect(testutil.CollectAndCompare(c, strings.NewReader(expected), fqName(MetricVMClassInfo))).To(Succeed())
	})
})
