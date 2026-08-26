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

package virtualmachine

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type stubIterator struct {
	vms []*v1alpha2.VirtualMachine
}

func (s stubIterator) Iter(_ context.Context, h handler) error {
	for _, vm := range s.vms {
		m := newDataMetric(vm)
		// The applied class is filled in by the real iterator, not by newDataMetric.
		m.AppliedVirtualMachineClassName = appliedClassName(vm)
		if stop := h(m); stop {
			return nil
		}
	}
	return nil
}

func collectorOf(vms ...*v1alpha2.VirtualMachine) Collector {
	return Collector{
		log:      log.NewNop(),
		iterator: stubIterator{vms: vms},
	}
}

// newVM builds the smallest machine that still produces every metric: no labels, annotations or
// pods, so that the dynamic series do not obscure the fixed ones.
func newVM(conditions ...metav1.Condition) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm-01", Namespace: "team-a", UID: types.UID("uid-vm-01")},
		Spec: v1alpha2.VirtualMachineSpec{
			VirtualMachineClassName: "generic",
			RunPolicy:               v1alpha2.AlwaysOnPolicy,
			CPU:                     v1alpha2.CPUSpec{Cores: 4, CoreFraction: "25%"},
			Memory:                  v1alpha2.MemorySpec{Size: resource.MustParse("2Gi")},
		},
		Status: v1alpha2.VirtualMachineStatus{
			Phase:      v1alpha2.MachineRunning,
			Node:       "node-1",
			Conditions: conditions,
			Resources: v1alpha2.ResourcesStatus{
				CPU:    v1alpha2.CPUStatus{Cores: 4, CoreFraction: "25%"},
				Memory: v1alpha2.MemoryStatus{},
			},
		},
	}
}

func migratable(status metav1.ConditionStatus, reason vmcondition.MigratableReason) metav1.Condition {
	return metav1.Condition{
		Type:   vmcondition.TypeMigratable.String(),
		Status: status,
		Reason: reason.String(),
	}
}

var _ = Describe("Collector", func() {
	It("reports every metric of a running machine", func() {
		c := collectorOf(newVM(migratable(metav1.ConditionTrue, vmcondition.ReasonMigratable)))

		Expect(testutil.CollectAndCompare(c, strings.NewReader(expectedRunningVM))).To(Succeed())
	})

	DescribeTable("reports whether the machine can be live migrated",
		func(condition metav1.Condition, value string) {
			c := collectorOf(newVM(condition))

			expected := `
# HELP d8_virtualization_virtualmachine_migratable Whether the virtualmachine can be live migrated.
# TYPE d8_virtualization_virtualmachine_migratable gauge
d8_virtualization_virtualmachine_migratable{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} ` + value + `
`
			Expect(testutil.CollectAndCompare(c, strings.NewReader(expected),
				"d8_virtualization_virtualmachine_migratable")).To(Succeed())
		},
		Entry("nothing blocks it",
			migratable(metav1.ConditionTrue, vmcondition.ReasonMigratable), "1"),
		// Local disks travel with the machine, so it stays migratable — this is the case the
		// KubeVirt metric got wrong.
		Entry("local disks travel with it",
			migratable(metav1.ConditionTrue, vmcondition.ReasonDisksShouldBeMigrating), "1"),
		Entry("local disks without volume migration",
			migratable(metav1.ConditionFalse, vmcondition.ReasonDisksNotMigratable), "0"),
		Entry("a host device blocks it",
			migratable(metav1.ConditionFalse, vmcondition.ReasonHostDevicesNotMigratable), "0"),
		Entry("anything else blocks it",
			migratable(metav1.ConditionFalse, vmcondition.ReasonNonMigratable), "0"),
	)

	// A machine that is not running carries no migratable condition: migratability is not evaluated
	// then. Exporting a zero would report "cannot be migrated" for every stopped machine, so the
	// series is omitted until there is an answer to report.
	It("exports no series for a machine whose migratable condition is not set", func() {
		c := collectorOf(newVM())

		Expect(testutil.CollectAndCompare(c, strings.NewReader(""),
			"d8_virtualization_virtualmachine_migratable")).To(Succeed())
	})
})

// expectedRunningVM is the full set of series a single running machine produces. Keeping the
// whole snapshot rather than one metric makes an accidental rename or label change fail here.
const expectedRunningVM = `
# HELP d8_virtualization_virtualmachine_agent_ready The virtualmachine agent ready.
# TYPE d8_virtualization_virtualmachine_agent_ready gauge
d8_virtualization_virtualmachine_agent_ready{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 0
# HELP d8_virtualization_virtualmachine_annotations Kubernetes annotations converted to Prometheus labels.
# TYPE d8_virtualization_virtualmachine_annotations gauge
d8_virtualization_virtualmachine_annotations{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 1
# HELP d8_virtualization_virtualmachine_awaiting_restart_to_apply_configuration The virtualmachine awaiting restart to apply configuration.
# TYPE d8_virtualization_virtualmachine_awaiting_restart_to_apply_configuration gauge
d8_virtualization_virtualmachine_awaiting_restart_to_apply_configuration{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 0
# HELP d8_virtualization_virtualmachine_configuration_applied The virtualmachine configuration applied.
# TYPE d8_virtualization_virtualmachine_configuration_applied gauge
d8_virtualization_virtualmachine_configuration_applied{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 1
# HELP d8_virtualization_virtualmachine_configuration_cpu_core_fraction The virtualmachine desired coreFraction from the spec.
# TYPE d8_virtualization_virtualmachine_configuration_cpu_core_fraction gauge
d8_virtualization_virtualmachine_configuration_cpu_core_fraction{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 25
# HELP d8_virtualization_virtualmachine_configuration_cpu_cores The virtualmachine desired core count from the spec.
# TYPE d8_virtualization_virtualmachine_configuration_cpu_cores gauge
d8_virtualization_virtualmachine_configuration_cpu_cores{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 4
# HELP d8_virtualization_virtualmachine_configuration_cpu_runtime_overhead The virtualmachine current cpu runtime overhead.
# TYPE d8_virtualization_virtualmachine_configuration_cpu_runtime_overhead gauge
d8_virtualization_virtualmachine_configuration_cpu_runtime_overhead{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 0
# HELP d8_virtualization_virtualmachine_configuration_memory_runtime_overhead_bytes The virtualmachine current memory runtime overhead.
# TYPE d8_virtualization_virtualmachine_configuration_memory_runtime_overhead_bytes gauge
d8_virtualization_virtualmachine_configuration_memory_runtime_overhead_bytes{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 0
# HELP d8_virtualization_virtualmachine_configuration_memory_size_bytes The virtualmachine current memory size.
# TYPE d8_virtualization_virtualmachine_configuration_memory_size_bytes gauge
d8_virtualization_virtualmachine_configuration_memory_size_bytes{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 2.147483648e+09
# HELP d8_virtualization_virtualmachine_configuration_run_policy The virtualmachine current runPolicy.
# TYPE d8_virtualization_virtualmachine_configuration_run_policy gauge
d8_virtualization_virtualmachine_configuration_run_policy{name="vm-01",namespace="team-a",node="node-1",runPolicy="AlwaysOff",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_configuration_run_policy{name="vm-01",namespace="team-a",node="node-1",runPolicy="AlwaysOn",uid="uid-vm-01"} 1
d8_virtualization_virtualmachine_configuration_run_policy{name="vm-01",namespace="team-a",node="node-1",runPolicy="AlwaysOnUnlessStoppedManually",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_configuration_run_policy{name="vm-01",namespace="team-a",node="node-1",runPolicy="Manual",uid="uid-vm-01"} 0
# HELP d8_virtualization_virtualmachine_cpu_core_fraction The virtualmachine current coreFraction.
# TYPE d8_virtualization_virtualmachine_cpu_core_fraction gauge
d8_virtualization_virtualmachine_cpu_core_fraction{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 25
# HELP d8_virtualization_virtualmachine_cpu_cores The virtualmachine current core count.
# TYPE d8_virtualization_virtualmachine_cpu_cores gauge
d8_virtualization_virtualmachine_cpu_cores{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 4
# HELP d8_virtualization_virtualmachine_firmware_up_to_date The virtualmachine firmware up to date.
# TYPE d8_virtualization_virtualmachine_firmware_up_to_date gauge
d8_virtualization_virtualmachine_firmware_up_to_date{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 1
# HELP d8_virtualization_virtualmachine_info Information about the virtualmachine.
# TYPE d8_virtualization_virtualmachine_info gauge
d8_virtualization_virtualmachine_info{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01",virtualmachineclass="generic"} 1
# HELP d8_virtualization_virtualmachine_labels Kubernetes labels converted to Prometheus labels.
# TYPE d8_virtualization_virtualmachine_labels gauge
d8_virtualization_virtualmachine_labels{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 1
# HELP d8_virtualization_virtualmachine_migratable Whether the virtualmachine can be live migrated.
# TYPE d8_virtualization_virtualmachine_migratable gauge
d8_virtualization_virtualmachine_migratable{name="vm-01",namespace="team-a",node="node-1",uid="uid-vm-01"} 1
# HELP d8_virtualization_virtualmachine_status_phase The virtualmachine current phase.
# TYPE d8_virtualization_virtualmachine_status_phase gauge
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Degraded",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Migrating",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Pause",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Pending",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Running",uid="uid-vm-01"} 1
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Starting",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Stopped",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Stopping",uid="uid-vm-01"} 0
d8_virtualization_virtualmachine_status_phase{name="vm-01",namespace="team-a",node="node-1",phase="Terminating",uid="uid-vm-01"} 0
`
