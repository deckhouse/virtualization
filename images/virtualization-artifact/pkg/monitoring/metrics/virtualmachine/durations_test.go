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
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestObserveLaunchStage(t *testing.T) {
	LaunchDuration.Reset()

	ObserveLaunchStage(LaunchStageWaitingForDependencies, 17*time.Second)
	ObserveLaunchStage(LaunchStageVirtualMachineStarting, 9*time.Second)
	ObserveLaunchStage(LaunchStageGuestOSAgentStarting, 13*time.Second)

	if got := testutil.CollectAndCount(LaunchDuration); got != 3 {
		t.Fatalf("expected one series per stage, got %d", got)
	}

	expected := `
# HELP d8_virtualization_virtualmachine_launch_duration_seconds The time a virtual machine spent in a launch stage.
# TYPE d8_virtualization_virtualmachine_launch_duration_seconds histogram
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="1"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="2"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="5"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="10"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="20"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="30"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="60"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="120"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="300"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="600"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="1800"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="3600"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="guest_os_agent_starting",le="+Inf"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_sum{stage="guest_os_agent_starting"} 13
d8_virtualization_virtualmachine_launch_duration_seconds_count{stage="guest_os_agent_starting"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="1"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="2"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="5"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="10"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="20"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="30"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="60"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="120"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="300"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="600"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="1800"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="3600"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="virtual_machine_starting",le="+Inf"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_sum{stage="virtual_machine_starting"} 9
d8_virtualization_virtualmachine_launch_duration_seconds_count{stage="virtual_machine_starting"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="1"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="2"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="5"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="10"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="20"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="30"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="60"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="120"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="300"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="600"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="1800"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="3600"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="+Inf"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_sum{stage="waiting_for_dependencies"} 17
d8_virtualization_virtualmachine_launch_duration_seconds_count{stage="waiting_for_dependencies"} 1
`
	err := testutil.CollectAndCompare(LaunchDuration, strings.NewReader(expected),
		"d8_virtualization_virtualmachine_launch_duration_seconds")
	if err != nil {
		t.Fatal(err)
	}
}

func TestObserveLaunchStageCountsZeroSkipsNegative(t *testing.T) {
	LaunchDuration.Reset()

	ObserveLaunchStage(LaunchStageWaitingForDependencies, 0)
	ObserveLaunchStage(LaunchStageWaitingForDependencies, -time.Second)

	expected := `
# HELP d8_virtualization_virtualmachine_launch_duration_seconds The time a virtual machine spent in a launch stage.
# TYPE d8_virtualization_virtualmachine_launch_duration_seconds histogram
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="1"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="2"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="5"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="10"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="20"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="30"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="60"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="120"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="300"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="600"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="1800"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="3600"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_bucket{stage="waiting_for_dependencies",le="+Inf"} 1
d8_virtualization_virtualmachine_launch_duration_seconds_sum{stage="waiting_for_dependencies"} 0
d8_virtualization_virtualmachine_launch_duration_seconds_count{stage="waiting_for_dependencies"} 1
`
	err := testutil.CollectAndCompare(LaunchDuration, strings.NewReader(expected),
		"d8_virtualization_virtualmachine_launch_duration_seconds")
	if err != nil {
		t.Fatal(err)
	}
}

func TestObserveMigration(t *testing.T) {
	MigrationDuration.Reset()

	ObserveMigration(MigrationTypeEvict, MigrationStageWaiting, MigrationResultSucceeded, 7*time.Second)
	ObserveMigration(MigrationTypeEvict, MigrationStagePreparing, MigrationResultSucceeded, 0)
	ObserveMigration(MigrationTypeEvict, MigrationStageTransfer, MigrationResultSucceeded, 4*time.Second)
	ObserveMigration(MigrationTypeMigrate, MigrationStageTransfer, MigrationResultFailed, -time.Second)

	count := func(migrationType, stage, result string) uint64 {
		h, err := MigrationDuration.GetMetricWithLabelValues(migrationType, stage, result)
		if err != nil {
			t.Fatal(err)
		}
		m := &dto.Metric{}
		if err := h.(prometheus.Metric).Write(m); err != nil {
			t.Fatal(err)
		}
		return m.GetHistogram().GetSampleCount()
	}

	if got := count(MigrationTypeEvict, MigrationStageWaiting, MigrationResultSucceeded); got != 1 {
		t.Errorf("waiting: expected one observation, got %d", got)
	}
	if got := count(MigrationTypeEvict, MigrationStagePreparing, MigrationResultSucceeded); got != 1 {
		t.Errorf("preparing: a stage that took less than a second is still an observation, got %d", got)
	}
	if got := count(MigrationTypeEvict, MigrationStageTransfer, MigrationResultSucceeded); got != 1 {
		t.Errorf("transfer: expected one observation, got %d", got)
	}
	if got := count(MigrationTypeMigrate, MigrationStageTransfer, MigrationResultFailed); got != 0 {
		t.Errorf("a negative duration must be dropped, got %d observations", got)
	}
}

func TestObserveShutdown(t *testing.T) {
	// A plain histogram cannot be reset, so the test counts the difference itself.
	before := shutdownCount(t)

	ObserveShutdown(12 * time.Second)
	ObserveShutdown(-time.Second)

	if got := shutdownCount(t) - before; got != 1 {
		t.Errorf("expected one observation and the negative one dropped, got %d", got)
	}
}

func shutdownCount(t *testing.T) uint64 {
	t.Helper()
	m := &dto.Metric{}
	if err := ShutdownDuration.Write(m); err != nil {
		t.Fatal(err)
	}
	return m.GetHistogram().GetSampleCount()
}
